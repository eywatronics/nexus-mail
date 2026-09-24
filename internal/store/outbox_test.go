package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func newOutbox(t *testing.T) (*Outbox, string) {
	t.Helper()
	dir := t.TempDir()
	return NewOutbox(dir), dir
}

func TestAMessageSurvivesTheRoundTripThroughTheOutbox(t *testing.T) {
	o, _ := newOutbox(t)
	raw := []byte("From: u@example.com\r\n\r\ngövde")

	name, err := o.Put(raw)
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	got, err := o.Get(name)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("the message came back as %q", got)
	}
}

// Two messages queued at once must not be the same file.
func TestEachMessageGetsItsOwnFile(t *testing.T) {
	o, _ := newOutbox(t)

	first, _ := o.Put([]byte("bir"))
	second, _ := o.Put([]byte("iki"))

	if first == second {
		t.Fatalf("both messages got the name %q", first)
	}
	a, _ := o.Get(first)
	b, _ := o.Get(second)
	if string(a) == string(b) {
		t.Error("the second message overwrote the first")
	}
}

// A crash halfway through a large write would otherwise leave a truncated file
// the worker would happily submit — and a message delivered with its last page
// missing is worse than one never sent, because nobody knows to send it again.
func TestPutLeavesNoTemporaryFileBehind(t *testing.T) {
	o, dir := newOutbox(t)

	if _, err := o.Put([]byte("metin")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("%s was left behind", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("the outbox holds %d files after one Put", len(entries))
	}
}

func TestAnEmptyMessageIsRefused(t *testing.T) {
	o, _ := newOutbox(t)

	if _, err := o.Put(nil); err == nil {
		t.Error("Put() queued an empty message")
	}
}

// The name comes out of the database, which makes it data this code did not
// write in this process. A stale or hand-edited row must not turn Get into a
// file read of the caller's choosing.
func TestAReferenceCannotEscapeTheOutbox(t *testing.T) {
	o, dir := newOutbox(t)

	// Something worth stealing, one level up.
	secret := filepath.Join(filepath.Dir(dir), "secret.eml")
	if err := os.WriteFile(secret, []byte("gizli"), 0o600); err != nil {
		t.Fatalf("writing the decoy: %v", err)
	}

	for _, name := range []string{
		"../secret.eml",
		`..\secret.eml`,
		"/etc/passwd",
		filepath.Join(dir, "a.eml"),
		"",
		"a.txt",
		"subdir/a.eml",
	} {
		if _, err := o.Get(name); err == nil {
			t.Errorf("Get(%q) reached outside the outbox", name)
		}
		if err := o.Remove(name); err == nil {
			t.Errorf("Remove(%q) reached outside the outbox", name)
		}
	}

	// And the decoy is still there.
	if _, err := os.Stat(secret); err != nil {
		t.Errorf("the file outside the outbox was touched: %v", err)
	}
}

// The worker deletes after the server accepted the message. A retry reaching
// this point twice has already done the only thing that mattered.
func TestRemovingAMessageTwiceIsNotAnError(t *testing.T) {
	o, _ := newOutbox(t)
	name, _ := o.Put([]byte("metin"))

	if err := o.Remove(name); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if err := o.Remove(name); err != nil {
		t.Errorf("removing a message that has gone is an error: %v", err)
	}
}

// Files accumulate two ways: a crash between writing the file and recording
// the row, and an operation the user cancelled. Both leave bytes nothing will
// ever read.
func TestSweepRemovesWhatNothingRefersTo(t *testing.T) {
	o, dir := newOutbox(t)

	keep, _ := o.Put([]byte("gonderilecek"))
	orphan, _ := o.Put([]byte("kimsenin beklemedigi"))

	// A rename that never happened.
	if err := os.WriteFile(filepath.Join(dir, "yarim.eml.tmp"), []byte("x"), 0o600); err != nil {
		t.Fatalf("writing the leftover: %v", err)
	}
	// Something that is not ours at all, which must be left alone.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("writing the stranger: %v", err)
	}

	removed, err := o.Sweep([]string{keep})
	if err != nil {
		t.Fatalf("Sweep() error: %v", err)
	}
	if removed != 2 {
		t.Errorf("Sweep() removed %d files, want the orphan and the leftover", removed)
	}

	if _, err := o.Get(keep); err != nil {
		t.Errorf("the message still queued was swept: %v", err)
	}
	if _, err := o.Get(orphan); err == nil {
		t.Error("the orphan survived")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Error("a file that was not ours was removed")
	}
}

// Sweep needs to know what is still queued, and the queue is the only thing
// that knows.
func TestPendingNamesComeFromTheQueue(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	if _, err := s.EnqueueOperation(ctx, model.Operation{
		AccountID: account, Kind: model.OpSend,
		Outbox: "abc.eml", EnvelopeFrom: "u@example.com",
		EnvelopeTo: []string{"r@example.com"}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("EnqueueOperation() error: %v", err)
	}

	names, err := s.PendingOutboxNames()
	if err != nil {
		t.Fatalf("PendingOutboxNames() error: %v", err)
	}
	if len(names) != 1 || names[0] != "abc.eml" {
		t.Errorf("PendingOutboxNames() = %v", names)
	}
}

// A send names no mailbox: the message has not reached one, and where the copy
// is filed is decided after the server accepts it.
func TestASendIsQueuedWithNoFolder(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	id, err := s.EnqueueOperation(ctx, model.Operation{
		AccountID: account, Kind: model.OpSend,
		Outbox: "abc.eml", EnvelopeFrom: "u@example.com",
		EnvelopeTo: []string{"r@example.com", "c@example.com"},
	})
	if err != nil {
		t.Fatalf("EnqueueOperation() error: %v", err)
	}

	var folderID *int64
	if err := s.Read().QueryRow(
		`SELECT folder_id FROM operations WHERE id = ?`, id).Scan(&folderID); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if folderID != nil {
		t.Errorf("folder_id = %d; a send belongs to no mailbox", *folderID)
	}

	claimed, err := s.ClaimOperations(ctx, account, 10)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d operations", len(claimed))
	}
	op := claimed[0]
	if op.Outbox != "abc.eml" {
		t.Errorf("Outbox = %q", op.Outbox)
	}
	if op.EnvelopeFrom != "u@example.com" || len(op.EnvelopeTo) != 2 {
		t.Errorf("the envelope did not survive: from=%q to=%v", op.EnvelopeFrom, op.EnvelopeTo)
	}
}

// The envelope is what makes delivery possible, and a blind copy is in it and
// in no header at all — so a send with none is one that reaches nobody.
func TestASendNeedsAFileAndAnEnvelope(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	cases := []struct {
		name string
		op   model.Operation
	}{
		{"no file", model.Operation{AccountID: account, Kind: model.OpSend,
			EnvelopeFrom: "u@example.com", EnvelopeTo: []string{"r@example.com"}}},
		{"no return path", model.Operation{AccountID: account, Kind: model.OpSend,
			Outbox: "a.eml", EnvelopeTo: []string{"r@example.com"}}},
		{"no recipients", model.Operation{AccountID: account, Kind: model.OpSend,
			Outbox: "a.eml", EnvelopeFrom: "u@example.com"}},
	}
	for _, c := range cases {
		if _, err := s.EnqueueOperation(ctx, c.op); err == nil {
			t.Errorf("%s: EnqueueOperation() accepted it", c.name)
		}
	}
}
