package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// seedFolderMessages puts UIDs into a folder with predictable subjects, so the
// tests below can assert on what survived.
func seedFolderMessages(t *testing.T, s *Store, acct, folderID int64, uids ...uint32) {
	t.Helper()

	msgs := make([]model.Message, 0, len(uids))
	for _, uid := range uids {
		msgs = append(msgs, model.Message{
			AccountID: acct, FolderID: folderID, UID: uid,
			Subject:      "Mesaj",
			InternalDate: time.Unix(int64(uid), 0),
			Flags:        []string{},
		})
	}
	if err := s.UpsertMessages(context.Background(), folderID, msgs); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
}

// Delta sync compares what the server still has against what we hold. Without
// the local UID set there is nothing to compare against, so this is the
// starting point of detecting a deletion.
func TestListMessageUIDsReturnsOneFoldersUIDsInOrder(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "Arşiv", Name: "Arşiv"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	other := folderIDByPath(t, s, acct, "Arşiv")

	seedFolderMessages(t, s, acct, inbox, 7, 3, 11)
	seedFolderMessages(t, s, acct, other, 99)

	got, err := s.ListMessageUIDs(ctx, inbox)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}

	// Ascending order is not cosmetic: the caller diffs this against the
	// server's UID list, and a sorted pair can be walked once instead of
	// building a set for a folder with fifty thousand messages.
	want := []uint32{3, 7, 11}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestListMessageUIDsIsEmptyForAnUntouchedFolder(t *testing.T) {
	s := openTestStore(t)
	_, inbox := seedInbox(t, s)

	got, err := s.ListMessageUIDs(context.Background(), inbox)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestSetMessageFlagsAppliesTheServersView(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1, 2)

	if err := s.SetMessageFlags(ctx, inbox, []model.FlagUpdate{
		{UID: 1, Flags: []string{model.FlagSeen, model.FlagFlagged}},
	}); err != nil {
		t.Fatalf("SetMessageFlags() error: %v", err)
	}

	msgs, err := s.ListMessages(ctx, inbox, 50, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	byUID := map[uint32]model.Message{}
	for _, m := range msgs {
		byUID[m.UID] = m
	}

	if !byUID[1].HasFlag(model.FlagSeen) || !byUID[1].HasFlag(model.FlagFlagged) {
		t.Errorf("UID 1 flags = %v, want seen and flagged", byUID[1].Flags)
	}
	// The update names one message. Rewriting the others would silently undo
	// state the server never mentioned.
	if len(byUID[2].Flags) != 0 {
		t.Errorf("UID 2 flags = %v, want them untouched", byUID[2].Flags)
	}
}

// The server reports flags for its whole mailbox; our retention window means
// we hold only part of it. A UID we never stored is not an error — it is the
// normal consequence of syncing a window.
func TestSetMessageFlagsIgnoresUIDsWeDoNotHold(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1)

	if err := s.SetMessageFlags(ctx, inbox, []model.FlagUpdate{
		{UID: 1, Flags: []string{model.FlagSeen}},
		{UID: 4242, Flags: []string{model.FlagSeen}},
	}); err != nil {
		t.Fatalf("SetMessageFlags() error: %v", err)
	}

	uids, err := s.ListMessageUIDs(ctx, inbox)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}
	if len(uids) != 1 || uids[0] != 1 {
		t.Errorf("the unknown UID was inserted; folder holds %v", uids)
	}
}

// A flag change fires the messages UPDATE trigger, which deletes and reinserts
// the FTS row. Wrong values there corrupt the index silently.
func TestSetMessageFlagsLeavesTheSearchIndexIntact(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, inbox, []model.Message{{
		AccountID: acct, FolderID: inbox, UID: 1,
		Subject: "Şubat mutabakatı", InternalDate: time.Unix(1, 0),
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	if err := s.SetMessageFlags(ctx, inbox, []model.FlagUpdate{
		{UID: 1, Flags: []string{model.FlagSeen}},
	}); err != nil {
		t.Fatalf("SetMessageFlags() error: %v", err)
	}

	got, err := s.SearchMessages(ctx, acct, "mutabakat", 50)
	if err != nil {
		t.Fatalf("SearchMessages() error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("the message is no longer searchable after a flag change: %d hits", len(got))
	}
	assertFTSIntegrity(t, s)
}

func TestDeleteMessagesByUIDRemovesOnlyWhatIsNamed(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "Arşiv", Name: "Arşiv"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	other := folderIDByPath(t, s, acct, "Arşiv")

	seedFolderMessages(t, s, acct, inbox, 1, 2, 3)
	// The same UID in another folder must survive: UIDs are unique per folder,
	// not per account, and deleting across folders is the classic bug.
	seedFolderMessages(t, s, acct, other, 2)

	if err := s.DeleteMessagesByUID(ctx, inbox, []uint32{2, 3}); err != nil {
		t.Fatalf("DeleteMessagesByUID() error: %v", err)
	}

	left, err := s.ListMessageUIDs(ctx, inbox)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}
	if len(left) != 1 || left[0] != 1 {
		t.Errorf("inbox holds %v, want just UID 1", left)
	}

	survivors, err := s.ListMessageUIDs(ctx, other)
	if err != nil {
		t.Fatalf("ListMessageUIDs(other) error: %v", err)
	}
	if len(survivors) != 1 {
		t.Errorf("the sibling folder lost UID 2; the delete crossed folders")
	}
}

// A deleted message must leave the search index too, or it stays findable
// after the server says it is gone.
func TestDeleteMessagesByUIDClearsTheSearchIndex(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, inbox, []model.Message{{
		AccountID: acct, FolderID: inbox, UID: 1,
		Subject: "Silinecek kayıt", InternalDate: time.Unix(1, 0),
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	if err := s.DeleteMessagesByUID(ctx, inbox, []uint32{1}); err != nil {
		t.Fatalf("DeleteMessagesByUID() error: %v", err)
	}

	got, err := s.SearchMessages(ctx, acct, "silinecek", 50)
	if err != nil {
		t.Fatalf("SearchMessages() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("the deleted message is still searchable: %d hits", len(got))
	}
	assertFTSIntegrity(t, s)
}

// Both take a list, and a delta sync that found nothing to change hands them
// an empty one on every pass. Neither may then touch the database.
func TestEmptyDeltaInputIsANoOp(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1)

	if err := s.SetMessageFlags(ctx, inbox, nil); err != nil {
		t.Errorf("SetMessageFlags(nil) error: %v", err)
	}
	if err := s.DeleteMessagesByUID(ctx, inbox, nil); err != nil {
		t.Errorf("DeleteMessagesByUID(nil) error: %v", err)
	}

	uids, err := s.ListMessageUIDs(ctx, inbox)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}
	if len(uids) != 1 {
		t.Errorf("an empty delta changed the folder; it now holds %v", uids)
	}
}

// Deleting in one statement per UID is what makes an expunge of ten thousand
// messages take minutes. The batch has to survive being handed a lot of them.
func TestDeleteMessagesByUIDHandlesALargeBatch(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	uids := make([]uint32, 0, 5000)
	for i := uint32(1); i <= 5000; i++ {
		uids = append(uids, i)
	}
	seedFolderMessages(t, s, acct, inbox, uids...)

	if err := s.DeleteMessagesByUID(ctx, inbox, uids); err != nil {
		t.Fatalf("DeleteMessagesByUID() error: %v", err)
	}

	left, err := s.ListMessageUIDs(ctx, inbox)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("%d messages survived the batch delete", len(left))
	}
	assertFTSIntegrity(t, s)
}
