package app

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func recipientFixture(t *testing.T) (*MailService, int64) {
	t.Helper()

	svc, accountID := sendFixture(t)
	ctx := context.Background()

	if err := svc.store.UpsertFolders(ctx, accountID, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	folders, err := svc.store.ListFolders(ctx, accountID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	folderID := folders[0].ID

	if err := svc.store.UpsertMessages(ctx, folderID, []model.Message{
		{
			AccountID: accountID, FolderID: folderID, UID: 1,
			From:         model.Address{Name: "Alice", Addr: "alice@example.com"},
			InternalDate: time.Unix(100, 0),
		},
		{
			AccountID: accountID, FolderID: folderID, UID: 2,
			From:         model.Address{Name: "Yazan", Addr: "u@example.com"},
			InternalDate: time.Unix(200, 0),
		},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	// An alias the account sends as is also one of the reader's own addresses.
	if _, err := svc.store.InsertIdentity(ctx, model.Identity{
		AccountID: accountID, Email: "destek@example.com", DisplayName: "Destek",
	}); err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}
	if err := svc.store.UpsertMessages(ctx, folderID, []model.Message{{
		AccountID: accountID, FolderID: folderID, UID: 3,
		From:         model.Address{Name: "Destek", Addr: "destek@example.com"},
		InternalDate: time.Unix(300, 0),
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	return svc, accountID
}

func TestSuggestRecipientsOmitsTheAccountsOwnAddresses(t *testing.T) {
	svc, accountID := recipientFixture(t)

	got, err := svc.SuggestRecipients(accountID, "example", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want only Alice: %+v", len(got), got)
	}
	if got[0].Addr != "alice@example.com" || got[0].Name != "Alice" {
		t.Errorf("got %+v, want Alice <alice@example.com>", got[0])
	}
}

func TestSuggestRecipientsHonoursTheLimit(t *testing.T) {
	svc, accountID := recipientFixture(t)

	got, err := svc.SuggestRecipients(accountID, "example", 1)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d suggestions with limit 1, want 1", len(got))
	}
}

// The window expects a list it can map over. A nil slice crosses the bridge as
// null, which is not the same shape and is the kind of thing that only shows
// up when the first keystroke returns nothing.
func TestSuggestRecipientsWithNoQueryReturnsAnEmptyList(t *testing.T) {
	svc, accountID := recipientFixture(t)

	got, err := svc.SuggestRecipients(accountID, "   ", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if got == nil {
		t.Fatal("SuggestRecipients() returned nil, want an empty list")
	}
	if len(got) != 0 {
		t.Errorf("got %d suggestions for blank input, want 0", len(got))
	}
}

func TestSuggestRecipientsRejectsAnUnknownAccount(t *testing.T) {
	svc, _ := recipientFixture(t)

	if _, err := svc.SuggestRecipients(9999, "example", 10); err == nil {
		t.Error("SuggestRecipients() invented an account for id 9999")
	}
}
