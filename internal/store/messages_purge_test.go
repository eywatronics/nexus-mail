package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// seedAged puts one message in a folder with a chosen age and flags.
func seedAged(t *testing.T, s *Store, acct, folderID int64, uid uint32, age time.Duration, flags ...string) {
	t.Helper()

	if flags == nil {
		flags = []string{}
	}
	if err := s.UpsertMessages(context.Background(), folderID, []model.Message{{
		AccountID: acct, FolderID: folderID, UID: uid,
		Subject:      "Eski kayıt",
		InternalDate: time.Now().Add(-age),
		Flags:        flags,
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
}

const (
	day   = 24 * time.Hour
	month = 30 * day
)

func TestPurgeRemovesMessagesPastTheAgeLimit(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	seedAged(t, s, acct, inbox, 1, 13*month)
	seedAged(t, s, acct, inbox, 2, 2*month)

	deleted, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{MaxAge: 12 * month})
	if err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("PurgeFolder() removed %d messages, want 1", deleted)
	}

	left := storedUIDsIn(t, s, inbox)
	if len(left) != 1 || left[0] != 2 {
		t.Errorf("folder holds %v, want only the recent message", left)
	}
}

func TestPurgeKeepsTheNewestMessagesPastTheCountLimit(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	// Five messages, newest first by age.
	for uid := uint32(1); uid <= 5; uid++ {
		seedAged(t, s, acct, inbox, uid, time.Duration(6-uid)*day)
	}

	deleted, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{MaxMessages: 3})
	if err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("PurgeFolder() removed %d messages, want 2", deleted)
	}

	left := storedUIDsIn(t, s, inbox)
	if len(left) != 3 || left[0] != 3 {
		t.Errorf("folder holds %v, want the three newest (UIDs 3, 4, 5)", left)
	}
}

// This is the rule that keeps the feature from being a betrayal: a message the
// user deliberately marked is not something to quietly delete for disk space.
func TestPurgeNeverRemovesStarredOrFlaggedMessages(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	seedAged(t, s, acct, inbox, 1, 20*month, model.FlagFlagged)
	seedAged(t, s, acct, inbox, 2, 20*month)
	// Servers differ on how they capitalise system flags, and the exemption
	// has to survive that.
	seedAged(t, s, acct, inbox, 3, 20*month, "\\FLAGGED")

	deleted, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{
		MaxAge: 12 * month, MaxMessages: 1,
	})
	if err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("PurgeFolder() removed %d messages, want only the unflagged one", deleted)
	}

	left := storedUIDsIn(t, s, inbox)
	if len(left) != 2 || left[0] != 1 || left[1] != 3 {
		t.Errorf("folder holds %v, want both flagged messages kept", left)
	}
}

// Local-first means the user decides what their disk holds. A policy with no
// limits must do nothing at all, not "nothing much".
func TestPurgeDoesNothingWhenTheWindowIsOff(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	for uid := uint32(1); uid <= 4; uid++ {
		seedAged(t, s, acct, inbox, uid, 40*month)
	}

	deleted, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{})
	if err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("PurgeFolder() removed %d messages with the window off", deleted)
	}
	if left := storedUIDsIn(t, s, inbox); len(left) != 4 {
		t.Errorf("folder holds %v, want all four", left)
	}
}

// The two limits are not a filter chain: a message is dropped when it fails
// either one, not only when it fails both.
func TestPurgeAppliesWhicheverLimitBitesFirst(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	seedAged(t, s, acct, inbox, 1, 20*month) // too old, but within the count
	seedAged(t, s, acct, inbox, 2, 3*day)    // recent, but pushed out by the count
	seedAged(t, s, acct, inbox, 3, 2*day)
	seedAged(t, s, acct, inbox, 4, 1*day)

	deleted, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{
		MaxAge: 12 * month, MaxMessages: 2,
	})
	if err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("PurgeFolder() removed %d messages, want 2", deleted)
	}

	left := storedUIDsIn(t, s, inbox)
	if len(left) != 2 || left[0] != 3 || left[1] != 4 {
		t.Errorf("folder holds %v, want the two newest", left)
	}
}

func TestPurgeIsScopedToOneFolder(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "Arşiv", Name: "Arşiv"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	other := folderIDByPath(t, s, acct, "Arşiv")

	seedAged(t, s, acct, inbox, 1, 20*month)
	seedAged(t, s, acct, other, 1, 20*month)

	if _, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{MaxAge: 12 * month}); err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}

	if left := storedUIDsIn(t, s, other); len(left) != 1 {
		t.Errorf("the sibling folder was purged too; it holds %v", left)
	}
}

// Bodies, attachments and the search index follow through the schema. If they
// did not, a purge would shrink the message list while leaving the bulk of the
// data on disk — the opposite of the point.
func TestPurgeTakesBodiesAndTheSearchIndexWithIt(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, inbox, []model.Message{{
		AccountID: acct, FolderID: inbox, UID: 1,
		Subject: "Silinecek mutabakat", InternalDate: time.Now().Add(-20 * month),
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	msgs, err := s.ListMessages(ctx, inbox, 10, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if err := s.SetMessageBody(ctx, msgs[0].ID, "<p>gövde</p>", "gövde"); err != nil {
		t.Fatalf("SetMessageBody() error: %v", err)
	}

	if _, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{MaxAge: 12 * month}); err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}

	if hits, _ := s.SearchMessages(ctx, acct, "mutabakat", 10); len(hits) != 0 {
		t.Errorf("the purged message is still searchable: %d hits", len(hits))
	}
	var bodies int
	if err := s.Read().QueryRow(`SELECT count(*) FROM message_bodies`).Scan(&bodies); err != nil {
		t.Fatalf("counting bodies: %v", err)
	}
	if bodies != 0 {
		t.Errorf("%d message bodies survived the purge", bodies)
	}
	assertFTSIntegrity(t, s)
}

// The purge is housekeeping on our own disk. Turning it into an IMAP delete
// would mean a client quietly destroying years of the user's mail on the
// server because their laptop was short of space.
func TestPurgeQueuesNothingForTheServer(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	seedAged(t, s, acct, inbox, 1, 20*month)

	if _, err := s.PurgeFolder(ctx, inbox, model.RetentionPolicy{MaxAge: 12 * month}); err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}

	var queued int
	if err := s.Read().QueryRow(`SELECT count(*) FROM operations`).Scan(&queued); err != nil {
		t.Fatalf("counting operations: %v", err)
	}
	if queued != 0 {
		t.Errorf("the purge queued %d operations for the server", queued)
	}
}

func TestPurgeOnAnEmptyFolder(t *testing.T) {
	s := openTestStore(t)
	_, inbox := seedInbox(t, s)

	deleted, err := s.PurgeFolder(context.Background(), inbox,
		model.RetentionPolicy{MaxAge: 12 * month, MaxMessages: 100})
	if err != nil {
		t.Fatalf("PurgeFolder() error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("PurgeFolder() removed %d messages from an empty folder", deleted)
	}
}

// storedUIDsIn is the UID list for one folder, ascending.
func storedUIDsIn(t *testing.T, s *Store, folderID int64) []uint32 {
	t.Helper()

	uids, err := s.ListMessageUIDs(context.Background(), folderID)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}
	return uids
}
