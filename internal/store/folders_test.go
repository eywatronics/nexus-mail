package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func seedAccount(t *testing.T, s *Store) int64 {
	t.Helper()
	id, err := s.InsertAccount(context.Background(), model.Account{
		Email: "seed@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "r", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

func folderIDByPath(t *testing.T, s *Store, accountID int64, path string) int64 {
	t.Helper()
	folders, err := s.ListFolders(context.Background(), accountID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	for _, f := range folders {
		if f.Path == path {
			return f.ID
		}
	}
	t.Fatalf("no folder with path %q", path)
	return 0
}

func TestUpsertFoldersIsIdempotentAndUpdatesCounts(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct := seedAccount(t, s)

	first := []model.Folder{
		{Path: "INBOX", Name: "INBOX", Delimiter: "/", UIDValidity: 10, UIDNext: 100, TotalCount: 5, UnreadCount: 2},
		{Path: "Sent", Name: "Sent", Delimiter: "/", Attributes: []string{"\\Sent"}, UIDValidity: 11},
	}
	if err := s.UpsertFolders(ctx, acct, first); err != nil {
		t.Fatalf("first UpsertFolders() error: %v", err)
	}

	// Re-running with changed counts must update rather than duplicate. This
	// runs on every sync, so getting it wrong would multiply the folder list.
	second := []model.Folder{
		{Path: "INBOX", Name: "INBOX", Delimiter: "/", UIDValidity: 10, UIDNext: 140, TotalCount: 9, UnreadCount: 3},
		{Path: "Sent", Name: "Sent", Delimiter: "/", Attributes: []string{"\\Sent"}, UIDValidity: 11},
	}
	if err := s.UpsertFolders(ctx, acct, second); err != nil {
		t.Fatalf("second UpsertFolders() error: %v", err)
	}

	got, err := s.ListFolders(ctx, acct)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListFolders() returned %d folders, want 2", len(got))
	}

	byPath := map[string]model.Folder{}
	for _, f := range got {
		byPath[f.Path] = f
	}
	inbox := byPath["INBOX"]
	if inbox.TotalCount != 9 {
		t.Errorf("INBOX TotalCount = %d, want 9", inbox.TotalCount)
	}
	if inbox.UnreadCount != 3 {
		t.Errorf("INBOX UnreadCount = %d, want 3", inbox.UnreadCount)
	}
	if inbox.UIDNext != 140 {
		t.Errorf("INBOX UIDNext = %d, want 140", inbox.UIDNext)
	}
	if !inbox.IsInbox() {
		t.Error("INBOX did not round-trip as the inbox")
	}

	sent := byPath["Sent"]
	if !sent.HasAttribute("\\Sent") {
		t.Errorf("Sent attributes = %v, want them to include \\Sent", sent.Attributes)
	}
}

func TestUpsertFoldersDefaultsTheDelimiter(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct := seedAccount(t, s)

	// Some servers report a NIL hierarchy delimiter for flat mailboxes; the
	// UI needs something to split paths on regardless.
	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "Flat", Name: "Flat"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	got, err := s.ListFolders(ctx, acct)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if got[0].Delimiter == "" {
		t.Error("Delimiter is empty; expected a default")
	}
}

func TestResetFolderDeletesMessagesAndSetsNewUIDValidity(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct := seedAccount(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{
		{Path: "INBOX", Name: "INBOX", UIDValidity: 10},
		{Path: "Other", Name: "Other", UIDValidity: 20},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	inboxID := folderIDByPath(t, s, acct, "INBOX")
	otherID := folderIDByPath(t, s, acct, "Other")

	for _, fid := range []int64{inboxID, otherID} {
		if err := s.UpsertMessages(ctx, fid, []model.Message{
			{AccountID: acct, FolderID: fid, UID: 1, Subject: "one", InternalDate: time.Unix(1, 0)},
			{AccountID: acct, FolderID: fid, UID: 2, Subject: "two", InternalDate: time.Unix(2, 0)},
		}); err != nil {
			t.Fatalf("UpsertMessages() error: %v", err)
		}
	}

	if err := s.ResetFolder(ctx, inboxID, 99); err != nil {
		t.Fatalf("ResetFolder() error: %v", err)
	}

	inboxMsgs, err := s.ListMessages(ctx, inboxID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(inbox) error: %v", err)
	}
	if len(inboxMsgs) != 0 {
		t.Errorf("inbox still holds %d messages after reset, want 0", len(inboxMsgs))
	}

	// The critical assertion: a UIDVALIDITY change invalidates one mailbox,
	// not the whole account. A sibling's cache is still correct.
	otherMsgs, err := s.ListMessages(ctx, otherID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(other) error: %v", err)
	}
	if len(otherMsgs) != 2 {
		t.Errorf("sibling folder holds %d messages, want 2 — the reset leaked across folders", len(otherMsgs))
	}

	folders, err := s.ListFolders(ctx, acct)
	if err != nil {
		t.Fatalf("ListFolders() after reset error: %v", err)
	}
	for _, f := range folders {
		switch f.ID {
		case inboxID:
			if f.UIDValidity != 99 {
				t.Errorf("inbox UIDValidity = %d, want 99", f.UIDValidity)
			}
			if f.HighestModSeq != 0 {
				t.Errorf("inbox HighestModSeq = %d, want 0 after reset", f.HighestModSeq)
			}
			if f.UIDNext != 0 {
				t.Errorf("inbox UIDNext = %d, want 0 after reset", f.UIDNext)
			}
		case otherID:
			if f.UIDValidity != 20 {
				t.Errorf("sibling UIDValidity = %d, want it unchanged at 20", f.UIDValidity)
			}
		}
	}
}
