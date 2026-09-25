package sync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/smtpx"
)

// fakeSender records what was submitted and fails on command.
type fakeSender struct {
	mu       sync.Mutex
	sent     []smtpx.Envelope
	bodies   [][]byte
	closed   int
	sendErr  error
	capsList smtpx.Capabilities
}

func (f *fakeSender) Capabilities() smtpx.Capabilities { return f.capsList }

func (f *fakeSender) Send(_ context.Context, env smtpx.Envelope, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, env)
	f.bodies = append(f.bodies, raw)
	return nil
}

func (f *fakeSender) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return nil
}

func (f *fakeSender) snapshot() ([]smtpx.Envelope, [][]byte, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]smtpx.Envelope(nil), f.sent...), append([][]byte(nil), f.bodies...), f.closed
}

// fakeOutbox is the bytes without the disk.
type fakeOutbox struct {
	mu      sync.Mutex
	files   map[string][]byte
	getErr  error
	removed []string
}

func newFakeOutbox(files map[string][]byte) *fakeOutbox {
	return &fakeOutbox{files: files}
}

func (o *fakeOutbox) Get(name string) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.getErr != nil {
		return nil, o.getErr
	}
	raw, ok := o.files[name]
	if !ok {
		return nil, fmt.Errorf("no such file %q", name)
	}
	return raw, nil
}

func (o *fakeOutbox) Remove(name string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.files, name)
	o.removed = append(o.removed, name)
	return nil
}

func (o *fakeOutbox) wasRemoved(name string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, n := range o.removed {
		if n == name {
			return true
		}
	}
	return false
}

// recordingStore captures how each operation was resolved.
type recordingStore struct {
	Store
	mu        sync.Mutex
	done      []int64
	failed    map[int64]string
	permanent map[int64]string
	// folders is what the sent copy is filed against. Empty means the account
	// has no Sent folder, which is a real configuration and not a broken one.
	folders []model.Folder
}

func newRecordingStore(inner Store) *recordingStore {
	return &recordingStore{Store: inner,
		failed: map[int64]string{}, permanent: map[int64]string{}}
}

func (r *recordingStore) ListFolders(context.Context, int64) ([]model.Folder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]model.Folder(nil), r.folders...), nil
}

func (r *recordingStore) MarkOperationDone(ctx context.Context, id int64) error {
	r.mu.Lock()
	r.done = append(r.done, id)
	r.mu.Unlock()
	return nil
}

func (r *recordingStore) MarkOperationFailed(ctx context.Context, id int64, reason string, _ time.Time) error {
	r.mu.Lock()
	r.failed[id] = reason
	r.mu.Unlock()
	return nil
}

func (r *recordingStore) MarkOperationPermanentlyFailed(ctx context.Context, id int64, reason string) error {
	r.mu.Lock()
	r.permanent[id] = reason
	r.mu.Unlock()
	return nil
}

func sendOp(id int64, file string) model.Operation {
	return model.Operation{
		ID: id, AccountID: 1, Kind: model.OpSend,
		Outbox: file, EnvelopeFrom: "u@example.com",
		EnvelopeTo: []string{"r@example.com"},
	}
}

// Everything else in the queue is an IMAP command against a folder; a send is
// not. Mixing them would hand a send to the IMAP applier, which has no branch
// for it.
func TestSendsAreSeparatedFromEverythingElse(t *testing.T) {
	ops := []model.Operation{
		{ID: 1, Kind: model.OpAddFlags},
		{ID: 2, Kind: model.OpSend},
		{ID: 3, Kind: model.OpMove},
		{ID: 4, Kind: model.OpSend},
		{ID: 5, Kind: model.OpEmptyFolder},
	}

	sends, rest := partitionSends(ops)

	if len(sends) != 2 || sends[0].ID != 2 || sends[1].ID != 4 {
		t.Errorf("sends = %v", ids(sends))
	}
	if len(rest) != 3 {
		t.Errorf("rest = %v", ids(rest))
	}
	// Order is preserved within each half: the queue is drained oldest first.
	if rest[0].ID != 1 || rest[1].ID != 3 || rest[2].ID != 5 {
		t.Errorf("the order changed: %v", ids(rest))
	}
}

func ids(ops []model.Operation) []int64 {
	var out []int64
	for _, op := range ops {
		out = append(out, op.ID)
	}
	return out
}

func TestPartitioningAnEmptyQueueYieldsNothing(t *testing.T) {
	sends, rest := partitionSends(nil)
	if len(sends) != 0 || len(rest) != 0 {
		t.Errorf("sends = %v, rest = %v", sends, rest)
	}
}

// An account configured for IMAP only is a legitimate one, and a send on it
// must fail with an explanation rather than a nil dereference.
func TestASendWithNoSubmissionServerFailsWithAnExplanation(t *testing.T) {
	e := New(newRecordingStore(nil), nil)
	rec := e.store.(*recordingStore)

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(7, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	reason, ok := rec.permanent[7]
	if !ok {
		t.Fatal("the send was not resolved")
	}
	if !strings.Contains(reason, "no submission server") {
		t.Errorf("the reason does not explain: %q", reason)
	}
}

// A submission server charges a handshake and an authentication round trip per
// connection; three messages written on a train are three messages, not three
// sessions.
func TestABatchOfMessagesGoesOverOneConnection(t *testing.T) {
	sender := &fakeSender{}
	outbox := newFakeOutbox(map[string][]byte{
		"a.eml": []byte("bir"), "b.eml": []byte("iki"), "c.eml": []byte("uc"),
	})

	dials := 0
	e := New(newRecordingStore(nil), nil)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) {
		dials++
		return sender, nil
	}, outbox)

	ops := []model.Operation{sendOp(1, "a.eml"), sendOp(2, "b.eml"), sendOp(3, "c.eml")}
	if err := e.drainSends(context.Background(), model.Account{ID: 1}, ops); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	if dials != 1 {
		t.Errorf("the batch opened %d connections", dials)
	}
	sent, bodies, closed := sender.snapshot()
	if len(sent) != 3 {
		t.Fatalf("%d messages were submitted", len(sent))
	}
	if string(bodies[0]) != "bir" || string(bodies[2]) != "uc" {
		t.Errorf("the wrong bytes were sent: %q", bodies)
	}
	if closed != 1 {
		t.Errorf("the connection was closed %d times", closed)
	}
}

// The envelope is what makes delivery happen, and a blind copy is in it and in
// no header at all.
func TestTheEnvelopeReachesTheSender(t *testing.T) {
	sender := &fakeSender{}
	e := New(newRecordingStore(nil), nil)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return sender, nil },
		newFakeOutbox(map[string][]byte{"a.eml": []byte("metin")}))

	op := sendOp(1, "a.eml")
	op.EnvelopeTo = []string{"r@example.com", "gizli@example.com"}

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{op}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	sent, _, _ := sender.snapshot()
	if len(sent) != 1 {
		t.Fatalf("%d messages were submitted", len(sent))
	}
	if sent[0].From != "u@example.com" || len(sent[0].To) != 2 {
		t.Errorf("the envelope arrived as %+v", sent[0])
	}
}

// A message the server will never accept, retried every few minutes, is a
// message that never leaves and never says so.
func TestAPermanentRefusalIsNotRetried(t *testing.T) {
	sender := &fakeSender{sendErr: fmt.Errorf("smtpx: %w: it is too large", smtpx.ErrRefused)}
	outbox := newFakeOutbox(map[string][]byte{"a.eml": []byte("metin")})

	e := New(newRecordingStore(nil), nil)
	rec := e.store.(*recordingStore)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return sender, nil }, outbox)

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(9, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	if _, ok := rec.permanent[9]; !ok {
		t.Error("a refusal the server will repeat was queued for retry")
	}
	// The file stays: the user may want to read it, fix it and send it again,
	// and this is the only copy in existence.
	if outbox.wasRemoved("a.eml") {
		t.Error("the refused message was deleted")
	}
}

// Giving up on a message the server never refused is the worse mistake: the
// user believes it was sent.
func TestATransientFailureIsRetriedAndKeepsTheMessage(t *testing.T) {
	sender := &fakeSender{sendErr: errors.New("connection reset by peer")}
	outbox := newFakeOutbox(map[string][]byte{"a.eml": []byte("metin")})

	e := New(newRecordingStore(nil), nil)
	rec := e.store.(*recordingStore)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return sender, nil }, outbox)

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(9, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	if reason, ok := rec.permanent[9]; ok {
		t.Errorf("a dropped connection was treated as permanent: %q", reason)
	}
	if outbox.wasRemoved("a.eml") {
		t.Error("the message was deleted although it was not sent")
	}
}

// The server may be down, the laptop may be on a train. Each message is
// retried on its own schedule rather than the batch being abandoned.
func TestADialFailureRetriesRatherThanGivingUp(t *testing.T) {
	e := New(newRecordingStore(nil), nil)
	rec := e.store.(*recordingStore)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) {
		return nil, errors.New("no route to host")
	}, newFakeOutbox(nil))

	ops := []model.Operation{sendOp(1, "a.eml"), sendOp(2, "b.eml")}
	if err := e.drainSends(context.Background(), model.Account{ID: 1}, ops); err != nil {
		t.Fatalf("drainSends() reported the batch as broken: %v", err)
	}

	for _, id := range []int64{1, 2} {
		if reason, ok := rec.permanent[id]; ok {
			t.Errorf("operation %d was given up on: %q", id, reason)
		}
	}
}

// Anything else would leave an operation failing forever against a file that
// does not exist.
func TestAMessageMissingFromTheOutboxIsNotRetriedForever(t *testing.T) {
	e := New(newRecordingStore(nil), nil)
	rec := e.store.(*recordingStore)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return &fakeSender{}, nil },
		newFakeOutbox(map[string][]byte{}))

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(4, "gone.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	reason, ok := rec.permanent[4]
	if !ok {
		t.Fatal("a message with no file was left to retry")
	}
	if !strings.Contains(reason, "no longer in the outbox") {
		t.Errorf("the reason does not say what happened: %q", reason)
	}
}

// Marked done before the file goes. The other order risks a crash between them
// leaving a queued operation whose message is gone, which the worker would
// then fail permanently for a message that was in fact delivered.
func TestASentMessageIsMarkedDoneAndThenRemoved(t *testing.T) {
	outbox := newFakeOutbox(map[string][]byte{"a.eml": []byte("metin")})
	e := New(newRecordingStore(nil), nil)
	rec := e.store.(*recordingStore)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return &fakeSender{}, nil },
		outbox)

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(5, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	if len(rec.done) != 1 || rec.done[0] != 5 {
		t.Errorf("done = %v", rec.done)
	}
	if !outbox.wasRemoved("a.eml") {
		t.Error("the sent message was left in the outbox")
	}
}

// An account whose queue holds only flag changes must not open a submission
// connection to find that out.
func TestNoSendsMeansNoSubmissionConnection(t *testing.T) {
	dials := 0
	e := New(newRecordingStore(nil), nil)
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) {
		dials++
		return &fakeSender{}, nil
	}, newFakeOutbox(nil))

	if err := e.drainSends(context.Background(), model.Account{ID: 1}, nil); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}
	if dials != 0 {
		t.Errorf("an empty batch opened %d connections", dials)
	}
}

// sentFolder is an account with somewhere to file the copy.
func sentFolder() []model.Folder {
	return []model.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", Name: "INBOX"},
		{ID: 2, AccountID: 1, Path: "Gönderilmiş Öğeler", Name: "Gönderilmiş Öğeler",
			Attributes: []string{"\\Sent"}},
	}
}

// Sending happens over SMTP and leaves no trace in the mailbox. Without this
// the message exists on the recipient's server and nowhere the writer can see
// it.
func TestASentMessageIsFiledInTheSentFolder(t *testing.T) {
	be := newFakeBackend()
	outbox := newFakeOutbox(map[string][]byte{"a.eml": []byte("From: u@example.com\r\n\r\nmetin")})

	e := New(newRecordingStore(nil), func(context.Context, int64) (imapx.MailBackend, error) {
		return be, nil
	})
	e.store.(*recordingStore).folders = sentFolder()
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return &fakeSender{}, nil },
		outbox)

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(1, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	filed := be.appendedMessages()
	if len(filed) != 1 {
		t.Fatalf("%d copies were filed", len(filed))
	}
	// The folder is found by role, not by name: a Turkish account's
	// "Gönderilmiş Öğeler" is as much the Sent folder as "Sent" is.
	if filed[0].mailbox != "Gönderilmiş Öğeler" {
		t.Errorf("the copy went to %q", filed[0].mailbox)
	}
	if !bytes.Equal(filed[0].raw, []byte("From: u@example.com\r\n\r\nmetin")) {
		t.Errorf("the filed copy is not the message that was sent: %q", filed[0].raw)
	}
	// The writer has read it: they wrote it. A Sent folder in bold is a folder
	// that looks like it needs attention.
	if len(filed[0].flags) != 1 || filed[0].flags[0] != model.FlagSeen {
		t.Errorf("the copy was filed with flags %v", filed[0].flags)
	}
}

// Some servers file the copy themselves, and appending to a folder we invented
// would leave a mailbox this client alone can see.
func TestWithNoSentFolderNothingIsFiledAndTheSendStillSucceeds(t *testing.T) {
	be := newFakeBackend()
	e := New(newRecordingStore(nil), func(context.Context, int64) (imapx.MailBackend, error) {
		return be, nil
	})
	// Folders, but none of them Sent.
	e.store.(*recordingStore).folders = []model.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", Name: "INBOX"},
	}
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return &fakeSender{}, nil },
		newFakeOutbox(map[string][]byte{"a.eml": []byte("metin")}))

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(2, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	if filed := be.appendedMessages(); len(filed) != 0 {
		t.Errorf("a copy was filed into %q with no Sent folder", filed[0].mailbox)
	}
	if done := e.store.(*recordingStore).done; len(done) != 1 {
		t.Errorf("the send was not recorded as done: %v", done)
	}
}

// The one thing that must not happen: a delivered message sent a second time.
//
// The copy is a convenience and the delivery is the point, so a Sent folder
// the server refused is a gap in the mailbox rather than a reason to retry.
func TestAFailedSentCopyDoesNotResendTheMessage(t *testing.T) {
	be := newFakeBackend()
	be.appendErr = errors.New("over quota")
	outbox := newFakeOutbox(map[string][]byte{"a.eml": []byte("metin")})

	e := New(newRecordingStore(nil), func(context.Context, int64) (imapx.MailBackend, error) {
		return be, nil
	})
	rec := e.store.(*recordingStore)
	rec.folders = sentFolder()
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) { return &fakeSender{}, nil },
		outbox)

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(3, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	if len(rec.done) != 1 || rec.done[0] != 3 {
		t.Errorf("done = %v; a failed copy must not undo a successful send", rec.done)
	}
	if _, retried := rec.failed[3]; retried {
		t.Error("the message was queued for retry, which would deliver it twice")
	}
	if _, gaveUp := rec.permanent[3]; gaveUp {
		t.Error("a delivered message was marked failed")
	}
	// And the outbox file goes: the message is delivered, so there is nothing
	// left to send.
	if !outbox.wasRemoved("a.eml") {
		t.Error("the delivered message was left in the outbox")
	}
}

// A copy of something that was never sent would be a Sent folder that lies.
func TestNothingIsFiledWhenTheSendItselfFailed(t *testing.T) {
	be := newFakeBackend()
	e := New(newRecordingStore(nil), func(context.Context, int64) (imapx.MailBackend, error) {
		return be, nil
	})
	e.store.(*recordingStore).folders = sentFolder()
	e.SetSender(func(context.Context, int64) (smtpx.MailSender, error) {
		return &fakeSender{sendErr: errors.New("connection reset")}, nil
	}, newFakeOutbox(map[string][]byte{"a.eml": []byte("metin")}))

	if err := e.drainSends(context.Background(), model.Account{ID: 1},
		[]model.Operation{sendOp(4, "a.eml")}); err != nil {
		t.Fatalf("drainSends() error: %v", err)
	}

	if filed := be.appendedMessages(); len(filed) != 0 {
		t.Error("a copy was filed for a message that never went out")
	}
}
