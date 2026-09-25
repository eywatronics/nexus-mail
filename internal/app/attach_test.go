package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nexusmail/internal/model"
)

// stubPicker answers with whatever the test says the person chose.
type stubPicker struct {
	files []string
	err   error
	calls int
}

func (p *stubPicker) PickFiles(string) ([]string, error) {
	p.calls++
	return p.files, p.err
}

// writeFile puts something on disk to be attached and returns its path.
func writeFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func TestAChosenFileIsDescribedWithoutBeingRead(t *testing.T) {
	svc, _ := sendFixture(t)
	path := writeFile(t, "rapor.pdf", []byte("%PDF-1.4 ..."))
	SetFilePicker(svc, &stubPicker{files: []string{path}})

	got, err := svc.PickAttachments()
	if err != nil {
		t.Fatalf("PickAttachments() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("PickAttachments() returned %d files", len(got))
	}
	if got[0].Name != "rapor.pdf" {
		t.Errorf("Name = %q", got[0].Name)
	}
	if got[0].Size != 12 {
		t.Errorf("Size = %d, want the file's 12 bytes", got[0].Size)
	}
	if got[0].MIMEType != "application/pdf" {
		t.Errorf("MIMEType = %q", got[0].MIMEType)
	}
}

// Cancelling a dialog is an answer, not a failure, and a composer that showed
// an error every time somebody changed their mind would be unusable.
func TestCancellingTheDialogIsNotAnError(t *testing.T) {
	svc, _ := sendFixture(t)
	SetFilePicker(svc, &stubPicker{files: nil})

	got, err := svc.PickAttachments()
	if err != nil {
		t.Fatalf("PickAttachments() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("PickAttachments() returned %d files after a cancel", len(got))
	}
}

func TestAnAttachmentReachesTheQueuedMessage(t *testing.T) {
	svc, account := sendFixture(t)
	path := writeFile(t, "notlar.txt", []byte("ekteki metin"))
	SetFilePicker(svc, &stubPicker{files: []string{path}})

	picked, err := svc.PickAttachments()
	if err != nil {
		t.Fatalf("PickAttachments() error: %v", err)
	}

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "metin",
		AttachmentPaths: []string{picked[0].Path},
	}); err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	_, raw := queued(t, svc, account)
	if !bytes.Contains(raw, []byte("notlar.txt")) {
		t.Errorf("the attachment is not named on the message:\n%s", raw)
	}
	// Base64, because the builder does not send arbitrary bytes as text.
	if !bytes.Contains(raw, []byte("ZWt0ZWtpIG1ldGlu")) {
		t.Errorf("the attachment's content is not on the message:\n%s", raw)
	}
}

// The window sends paths, not bytes. Without a record of what the dialog
// handed out, that pair of methods would amount to "the window may ask the
// backend to read any file on this machine and mail it somewhere".
func TestAPathNobodyChoseIsRefused(t *testing.T) {
	svc, account := sendFixture(t)
	secret := writeFile(t, "gizli.txt", []byte("parola"))

	_, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "metin",
		AttachmentPaths: []string{secret},
	})
	if err == nil {
		t.Fatal("SendMessage() attached a file that was never chosen")
	}
	if !strings.Contains(err.Error(), "file dialog") {
		t.Errorf("the error does not say why: %v", err)
	}
}

// A file that was chosen and then deleted stops the send. Queuing a message
// whose attachment is missing would produce mail with a paperclip the reader
// cannot open, and nothing would say so.
func TestAFileThatVanishedAfterBeingChosenStopsTheSend(t *testing.T) {
	svc, account := sendFixture(t)
	path := writeFile(t, "gecici.txt", []byte("bir sey"))
	SetFilePicker(svc, &stubPicker{files: []string{path}})

	if _, err := svc.PickAttachments(); err != nil {
		t.Fatalf("PickAttachments() error: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing the file: %v", err)
	}

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "metin",
		AttachmentPaths: []string{path},
	}); err == nil {
		t.Error("SendMessage() queued a message whose attachment is gone")
	}

	// And nothing was queued: a failed send must not leave a half message.
	ops, err := svc.store.ClaimOperations(context.Background(), account, 10)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	for _, op := range ops {
		if op.Kind == model.OpSend {
			t.Error("a send was queued despite the failure")
		}
	}
}

// A folder is not a file, and attaching nothing while saying nothing would be
// worse than the error.
func TestAFolderIsRefusedAtTheDialog(t *testing.T) {
	svc, _ := sendFixture(t)
	SetFilePicker(svc, &stubPicker{files: []string{t.TempDir()}})

	if _, err := svc.PickAttachments(); err == nil {
		t.Error("PickAttachments() accepted a folder")
	}
}

// The cap is about this process's memory, not about any protocol: the whole
// message, base64 and all, is held here while it is built.
func TestAttachmentsBeyondTheCapAreRefused(t *testing.T) {
	svc, account := sendFixture(t)
	big := writeFile(t, "buyuk.bin", bytes.Repeat([]byte("x"), maxAttachmentBytes+1))
	SetFilePicker(svc, &stubPicker{files: []string{big}})

	if _, err := svc.PickAttachments(); err != nil {
		t.Fatalf("PickAttachments() error: %v", err)
	}

	_, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "metin",
		AttachmentPaths: []string{big},
	})
	if err == nil {
		t.Fatal("SendMessage() accepted more than the cap")
	}
	if !strings.Contains(err.Error(), "MB") {
		t.Errorf("the error does not say what the limit is: %v", err)
	}
}

// A build with no dialog says so rather than dereferencing nothing.
func TestPickingWithNoDialogIsReported(t *testing.T) {
	svc, _ := sendFixture(t)

	if _, err := svc.PickAttachments(); err == nil {
		t.Error("PickAttachments() worked without a file dialog")
	}
}

// An extension nothing recognises leaves the type off, and the builder then
// declares the generic binary one. A wrong type tells the reader's client to
// open the file with the wrong thing.
func TestAnUnknownExtensionGetsNoGuess(t *testing.T) {
	if got := mimeTypeOf("/tmp/veri.zzzbilinmeyen"); got != "" {
		t.Errorf("mimeTypeOf() = %q, want no guess", got)
	}
}

// The extension table hands back parameters the builder sets itself.
func TestTheGuessedTypeCarriesNoParameters(t *testing.T) {
	got := mimeTypeOf("/tmp/notlar.txt")
	if got != "text/plain" {
		t.Errorf("mimeTypeOf() = %q, want text/plain with nothing after it", got)
	}
}
