package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// rawBackend hands out a message whose bytes the test chooses, so the source
// view and the .eml save can be checked against something that is not valid
// UTF-8 — which is the interesting case, not the easy one.
type rawBackend struct {
	stubBackend
	raw []byte
}

func (b rawBackend) FetchRaw(context.Context, uint32) ([]byte, error) {
	return b.raw, nil
}

// firstMessage syncs one account and returns the id of the message in its
// inbox, which is what every test below starts from.
func firstMessage(t *testing.T, svc *MailService) int64 {
	t.Helper()

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	var inbox FolderDTO
	for _, f := range folders {
		if f.IsInbox {
			inbox = f
		}
	}
	msgs, err := svc.ListMessages(inbox.ID, 10, 0, false)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("ListMessages() = %v, %v", msgs, err)
	}
	return msgs[0].ID
}

// An .eml is only worth saving if it is the message byte for byte. A file this
// client had re-encoded on the way out would open in another mail client and
// show something the sender never sent.
func TestSaveMessageAsEMLWritesTheBytesUntouched(t *testing.T) {
	// Latin-5 bytes: this is deliberately not valid UTF-8.
	raw := []byte("Subject: Rapor\r\nContent-Type: text/plain; charset=iso-8859-9\r\n\r\n" +
		"Mutabakat\xfd s\xf6zle\xfemesi")

	svc, _, _ := newTestService(t, rawBackend{raw: raw})
	dir := t.TempDir()
	svc.cfg.AttachmentDir = func() (string, error) { return dir, nil }
	id := firstMessage(t, svc)

	path, err := svc.SaveMessageAsEML(id)
	if err != nil {
		t.Fatalf("SaveMessageAsEML() error: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("saved to %q, want a file inside %q", path, dir)
	}
	if !strings.HasSuffix(path, ".eml") {
		t.Errorf("saved as %q, want an .eml", path)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("the saved file differs from what arrived:\n got %q\nwant %q", got, raw)
	}
}

// The filename carries the subject (the synced stub's is "Hello") so a folder of
// saved mail is readable, and
// it carries the message id so two mails with the same subject cannot
// overwrite each other.
func TestSavedEMLIsNamedAfterTheSubject(t *testing.T) {
	svc, _, _ := newTestService(t, rawBackend{raw: []byte("Subject: x\r\n\r\nx")})
	dir := t.TempDir()
	svc.cfg.AttachmentDir = func() (string, error) { return dir, nil }
	id := firstMessage(t, svc)

	path, err := svc.SaveMessageAsEML(id)
	if err != nil {
		t.Fatalf("SaveMessageAsEML() error: %v", err)
	}
	name := filepath.Base(path)
	if !strings.Contains(name, "Hello") {
		t.Errorf("filename = %q, want the subject in it", name)
	}
}

// A subject is attacker-controlled text, exactly like an attachment filename,
// and it is about to be joined to a path.
func TestEMLFilenameCannotEscapeTheDirectory(t *testing.T) {
	name := emlFilename(7, "../../../etc/passwd")
	if strings.ContainsAny(name, "/"+string(filepath.Separator)) {
		t.Errorf("emlFilename() = %q, which still names a path", name)
	}
}

// A mailing-list subject can run past the length limit a filesystem imposes,
// and the write would fail on a message that is otherwise fine.
func TestEMLFilenameIsCappedWithoutCuttingARune(t *testing.T) {
	name := emlFilename(1, strings.Repeat("ğ", 400))
	if len([]rune(name)) > 100 {
		t.Errorf("emlFilename() produced a %d-rune name", len([]rune(name)))
	}
	for _, r := range name {
		if r == 0xFFFD {
			t.Error("the cap cut through the middle of a character")
		}
	}
}

// A message with no subject still has to get a name.
func TestEMLFilenameFallsBackWhenThereIsNoSubject(t *testing.T) {
	if name := emlFilename(3, "   "); name != "3-message.eml" {
		t.Errorf("emlFilename() = %q for an empty subject", name)
	}
}

// The source view is what a reader opens when the message rendered wrong. It
// has to be the original, and it has to be inert: text/plain, never markup.
func TestSourceViewServesTheMessageAsPlainText(t *testing.T) {
	raw := []byte("Subject: Kaynak\r\nContent-Type: text/html\r\n\r\n<script>alert(1)</script>")

	svc, _, _ := newTestService(t, rawBackend{raw: raw})
	h := NewBodyHandler(svc)
	id := firstMessage(t, svc)

	rec := get(t, h, fmt.Sprintf("%s%d", sourcePath, id))
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nosniff is missing, so a browser may sniff the script tag into markup")
	}
	if !strings.Contains(rec.Body.String(), "Subject: Kaynak") {
		t.Errorf("the source view does not show the headers:\n%s", rec.Body.String())
	}
}

// A message in Latin-5 is not valid UTF-8, and serving its bytes under a
// charset that says otherwise leaves the browser to guess. The replacement
// character is the honest answer: these bytes were not text in this encoding.
// The untouched bytes are still one click away as an .eml.
func TestSourceViewDoesNotClaimInvalidBytesAreUTF8(t *testing.T) {
	raw := []byte{'S', 'u', 'b', 'j', ':', ' ', 0xFD, 0xFE, '@'}

	svc, _, _ := newTestService(t, rawBackend{raw: raw})
	h := NewBodyHandler(svc)
	id := firstMessage(t, svc)

	body := get(t, h, fmt.Sprintf("%s%d", sourcePath, id)).Body.Bytes()
	if !utf8.Valid(body) {
		t.Errorf("the source view served %q under charset=utf-8", body)
	}
	if !strings.HasPrefix(string(body), "Subj: ") {
		t.Errorf("the valid part of the message was lost: %q", body)
	}
}

func TestSourceViewRejectsAMalformedPath(t *testing.T) {
	svc, _, _ := newTestService(t, rawBackend{raw: []byte("x")})
	h := NewBodyHandler(svc)

	if rec := get(t, h, sourcePath+"not-a-number"); rec.Code != 400 {
		t.Errorf("status = %d for a malformed id, want 400", rec.Code)
	}
}

func TestServiceKeepsARawFetchOutOfTheDatabase(t *testing.T) {
	raw := []byte("Subject: x\r\n\r\nCANARY-RAW-BODY")

	svc, _, s := newTestService(t, rawBackend{raw: raw})
	dir := t.TempDir()
	svc.cfg.AttachmentDir = func() (string, error) { return dir, nil }
	id := firstMessage(t, svc)

	if _, err := svc.SaveMessageAsEML(id); err != nil {
		t.Fatalf("SaveMessageAsEML() error: %v", err)
	}

	// The raw form is deliberately not cached: it would be a second full copy
	// of every message the reader ever looked at the source of.
	var found int
	if err := s.Read().QueryRow(
		`SELECT count(*) FROM message_bodies WHERE html_body LIKE ? OR text_body LIKE ?`,
		"%CANARY-RAW-BODY%", "%CANARY-RAW-BODY%").Scan(&found); err != nil {
		t.Fatalf("querying the body table: %v", err)
	}
	if found != 0 {
		t.Errorf("the raw message was cached in %d rows", found)
	}
}
