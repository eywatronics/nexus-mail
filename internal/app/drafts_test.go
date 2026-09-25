package app

import (
	"errors"
	"os"
	"testing"

	"nexusmail/internal/store"
)

// The worst thing a mail client can do is lose the one part of a message the
// person made themselves.
func TestADraftSurvivesAndComesBackAsItWasTyped(t *testing.T) {
	svc, account := sendFixture(t)

	id, err := svc.SaveDraft(DraftRecordDTO{
		AccountID: account,
		// Half-typed, which is the normal state of a draft and the state a
		// parse-and-reassemble round trip would not survive.
		To:      "ali@example.com, bir yar",
		Subject: "Yarim konu",
		Body:    "burada kalmis",
	})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}

	got, err := svc.Draft(id)
	if err != nil {
		t.Fatalf("Draft() error: %v", err)
	}
	if got.To != "ali@example.com, bir yar" {
		t.Errorf("To = %q; the address line was not kept as typed", got.To)
	}
	if got.Body != "burada kalmis" {
		t.Errorf("Body = %q", got.Body)
	}
}

// Autosave has to write one row, not one row per pause.
func TestSavingTheSameDraftAgainReplacesIt(t *testing.T) {
	svc, account := sendFixture(t)

	id, err := svc.SaveDraft(DraftRecordDTO{AccountID: account, Body: "ilk"})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}
	again, err := svc.SaveDraft(DraftRecordDTO{ID: id, AccountID: account, Body: "ikinci"})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}
	if again != id {
		t.Errorf("the second save made draft %d instead of replacing %d", again, id)
	}

	list, err := svc.Drafts(account)
	if err != nil {
		t.Fatalf("Drafts() error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("the account has %d drafts after two saves of one", len(list))
	}
	if list[0].Body != "ikinci" {
		t.Errorf("Body = %q, want the newer text", list[0].Body)
	}
}

// A draft left behind after the message went out is a message the person will
// send a second time.
func TestSendingClearsTheDraftItCameFrom(t *testing.T) {
	svc, account := sendFixture(t)

	id, err := svc.SaveDraft(DraftRecordDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Body: "metin",
	})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "metin",
		DraftID: id,
	}); err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	list, err := svc.Drafts(account)
	if err != nil {
		t.Fatalf("Drafts() error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("%d drafts survived the send", len(list))
	}
}

// And a send that was refused must not take the draft with it: the message is
// still only in the composer and in this row.
func TestARefusedSendKeepsTheDraft(t *testing.T) {
	svc, account := sendFixture(t)

	id, err := svc.SaveDraft(DraftRecordDTO{AccountID: account, To: "bozuk adres"})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "bozuk <<< adres", Subject: "Konu", DraftID: id,
	}); err == nil {
		t.Fatal("SendMessage() accepted an unparseable address")
	}

	if _, err := svc.Draft(id); err != nil {
		t.Errorf("the draft was thrown away with the failed send: %v", err)
	}
}

// Discarding twice is not an error. Two windows on one draft would otherwise
// fail the second time, after the draft was already gone.
func TestDiscardingADraftTwiceIsNotAnError(t *testing.T) {
	svc, account := sendFixture(t)

	id, err := svc.SaveDraft(DraftRecordDTO{AccountID: account, Body: "bir sey"})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}
	if err := svc.DiscardDraft(id); err != nil {
		t.Fatalf("DiscardDraft() error: %v", err)
	}
	if err := svc.DiscardDraft(id); err != nil {
		t.Errorf("discarding an already discarded draft failed: %v", err)
	}
}

// Saving into a draft that is no longer there must not read as a successful
// save: the composer would go on believing its work was safe.
func TestSavingIntoADraftThatIsGoneIsReported(t *testing.T) {
	svc, account := sendFixture(t)

	id, err := svc.SaveDraft(DraftRecordDTO{AccountID: account, Body: "bir sey"})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}
	if err := svc.DiscardDraft(id); err != nil {
		t.Fatalf("DiscardDraft() error: %v", err)
	}

	_, err = svc.SaveDraft(DraftRecordDTO{ID: id, AccountID: account, Body: "devami"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SaveDraft() into a deleted draft returned %v", err)
	}
}

// A draft with no account belongs to nobody and could never be opened again.
func TestADraftWithNoAccountIsRefused(t *testing.T) {
	svc, _ := sendFixture(t)

	if _, err := svc.SaveDraft(DraftRecordDTO{Body: "bir sey"}); err == nil {
		t.Error("SaveDraft() accepted a draft with no account")
	}
}

// Attachments are remembered by path, and the paths still have to be ones the
// dialog handed out when the draft is finally sent.
func TestADraftRemembersItsAttachmentsByPath(t *testing.T) {
	svc, account := sendFixture(t)

	path := writeFile(t, "ek.txt", []byte("icerik"))
	id, err := svc.SaveDraft(DraftRecordDTO{
		AccountID: account, AttachmentPaths: []string{path},
	})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}

	got, err := svc.Draft(id)
	if err != nil {
		t.Fatalf("Draft() error: %v", err)
	}
	// Described, so the composer can show a name and a size without reading
	// the file.
	if len(got.Attachments) != 1 || got.Attachments[0].Name != "ek.txt" {
		t.Fatalf("Attachments = %+v", got.Attachments)
	}
	if got.Attachments[0].Size != 6 {
		t.Errorf("Size = %d, want the file's 6 bytes", got.Attachments[0].Size)
	}

	// And sendable without going through the dialog again. The path came out
	// of this application's own database, having been chosen in the dialog
	// when the draft was written; refusing it would mean a draft silently
	// comes back with one fewer attachment.
	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", DraftID: id,
		AttachmentPaths: []string{path},
	}); err != nil {
		t.Errorf("a reopened draft could not send its own attachment: %v", err)
	}
}

// What the allowlist keeps out is a path the window made up, and reopening a
// draft must not become a way to launder one.
func TestAPathTheWindowInventedIsStillRefusedAfterOpeningADraft(t *testing.T) {
	svc, account := sendFixture(t)

	chosen := writeFile(t, "ek.txt", []byte("icerik"))
	id, err := svc.SaveDraft(DraftRecordDTO{
		AccountID: account, AttachmentPaths: []string{chosen},
	})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}
	if _, err := svc.Draft(id); err != nil {
		t.Fatalf("Draft() error: %v", err)
	}

	secret := writeFile(t, "gizli.txt", []byte("parola"))
	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", DraftID: id,
		AttachmentPaths: []string{secret},
	}); err == nil {
		t.Error("opening a draft admitted a path nobody chose")
	}
}

// A draft written last week whose attachment has since been moved still opens.
// The words in it are worth more than the file, and what is gone is named
// rather than dropped in silence.
func TestAMissingAttachmentIsNamedRatherThanDropped(t *testing.T) {
	svc, account := sendFixture(t)

	path := writeFile(t, "gitmis.txt", []byte("icerik"))
	id, err := svc.SaveDraft(DraftRecordDTO{
		AccountID: account, Body: "onemli metin", AttachmentPaths: []string{path},
	})
	if err != nil {
		t.Fatalf("SaveDraft() error: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing the file: %v", err)
	}

	got, err := svc.Draft(id)
	if err != nil {
		t.Fatalf("the draft could not be opened at all: %v", err)
	}
	if got.Body != "onemli metin" {
		t.Errorf("Body = %q", got.Body)
	}
	if len(got.Attachments) != 0 {
		t.Errorf("Attachments = %+v, want none", got.Attachments)
	}
	if len(got.MissingAttachments) != 1 || got.MissingAttachments[0] != "gitmis.txt" {
		t.Errorf("MissingAttachments = %v", got.MissingAttachments)
	}
}
