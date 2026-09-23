package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func ftsMatchCount(t *testing.T, s *Store, query string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow(
		`SELECT count(*) FROM fts_messages WHERE fts_messages MATCH ?`, query).Scan(&n); err != nil {
		t.Fatalf("FTS match %q: %v", query, err)
	}
	return n
}

// assertFTSIntegrity is the only real proof that the 'delete' commands carried
// the values as they were indexed. A corrupted FTS index answers queries
// wrongly but silently; only this check shouts.
func assertFTSIntegrity(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.Write().Exec(
		`INSERT INTO fts_messages(fts_messages) VALUES('integrity-check')`); err != nil {
		t.Errorf("FTS integrity check failed: %v", err)
	}
}

func TestTriggersKeepTheSearchIndexInStep(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct := seedAccount(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{
		{Path: "INBOX", Name: "INBOX"},
		{Path: "Other", Name: "Other"},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	inboxID := folderIDByPath(t, s, acct, "INBOX")
	otherID := folderIDByPath(t, s, acct, "Other")

	msg := model.Message{
		AccountID: acct, FolderID: inboxID, UID: 1,
		Subject: "Quarterly invoice", From: model.Address{Addr: "billing@example.com"},
		Snippet: "attached please find", InternalDate: time.Unix(1, 0),
	}
	if err := s.UpsertMessages(ctx, inboxID, []model.Message{msg}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	if got := ftsMatchCount(t, s, "invoice"); got != 1 {
		t.Errorf("after insert, matches for 'invoice' = %d, want 1", got)
	}
	// The sender address is indexed too, so from: search works in P3.
	if got := ftsMatchCount(t, s, "billing"); got != 1 {
		t.Errorf("after insert, matches for 'billing' = %d, want 1", got)
	}

	// An upsert fires the UPDATE trigger, not INSERT. Miss that distinction
	// and the index accumulates stale rows, so search returns both spellings.
	msg.Subject = "Quarterly receipt"
	if err := s.UpsertMessages(ctx, inboxID, []model.Message{msg}); err != nil {
		t.Fatalf("second UpsertMessages() error: %v", err)
	}
	if got := ftsMatchCount(t, s, "receipt"); got != 1 {
		t.Errorf("after upsert, matches for 'receipt' = %d, want 1", got)
	}
	if got := ftsMatchCount(t, s, "invoice"); got != 0 {
		t.Errorf("the old subject still matches %d times; the index kept a stale row", got)
	}

	// A sibling folder's entries must survive a reset of its neighbour.
	sibling := msg
	sibling.FolderID, sibling.UID, sibling.Subject = otherID, 2, "Sibling notice"
	if err := s.UpsertMessages(ctx, otherID, []model.Message{sibling}); err != nil {
		t.Fatalf("UpsertMessages(sibling) error: %v", err)
	}
	if err := s.ResetFolder(ctx, inboxID, 42); err != nil {
		t.Fatalf("ResetFolder() error: %v", err)
	}
	if got := ftsMatchCount(t, s, "receipt"); got != 0 {
		t.Errorf("reset folder still has %d index entries, want 0", got)
	}
	if got := ftsMatchCount(t, s, "notice"); got != 1 {
		t.Errorf("sibling folder lost its index entry; the reset leaked")
	}

	assertFTSIntegrity(t, s)
}

// Deleting an account cascades to its messages, and each cascaded delete must
// still fire the FTS delete trigger. If it does not, removing an account
// leaves its mail searchable.
func TestCascadingDeleteAlsoClearsTheSearchIndex(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct := seedAccount(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	fid := folderIDByPath(t, s, acct, "INBOX")
	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "Confidential memo", InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	if got := ftsMatchCount(t, s, "confidential"); got != 1 {
		t.Fatalf("precondition failed: matches = %d, want 1", got)
	}

	if _, err := s.Write().ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, acct); err != nil {
		t.Fatalf("delete account: %v", err)
	}

	if got := ftsMatchCount(t, s, "confidential"); got != 0 {
		t.Errorf("removed account's mail is still searchable: %d matches", got)
	}
	assertFTSIntegrity(t, s)
}

// Turkish subjects exercise Unicode handling: FTS5's default tokenizer needs
// unicode61 behaviour for these to match at all, and Turkish is this project's
// primary user language.
func TestSearchIndexHandlesTurkishSubjects(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct := seedAccount(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	fid := folderIDByPath(t, s, acct, "INBOX")

	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "Şubat ayı faturası", InternalDate: time.Unix(1, 0)},
		{AccountID: acct, FolderID: fid, UID: 2, Subject: "Toplantı notları", InternalDate: time.Unix(2, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	for _, term := range []string{"faturası", "Şubat", "notları"} {
		if got := ftsMatchCount(t, s, term); got != 1 {
			t.Errorf("matches for %q = %d, want 1", term, got)
		}
	}
	assertFTSIntegrity(t, s)
}
