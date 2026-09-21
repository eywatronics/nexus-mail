package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func messageIDsOf(t *testing.T, s *Store, folderID int64) map[uint32]int64 {
	t.Helper()

	msgs, err := s.ListMessages(context.Background(), folderID, 500, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	out := map[uint32]int64{}
	for _, m := range msgs {
		out[m.UID] = m.ID
	}
	return out
}

func flagsOf(t *testing.T, s *Store, folderID int64, uid uint32) []string {
	t.Helper()

	msgs, err := s.ListMessages(context.Background(), folderID, 500, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	for _, m := range msgs {
		if m.UID == uid {
			return m.Flags
		}
	}
	t.Fatalf("no message with UID %d", uid)
	return nil
}

// The local write and the queued intent have to happen together. Half of this
// leaves the window showing a message as read that the server will never hear
// about, and the user finds out on their phone a week later.
func TestApplyFlagChangeWritesLocallyAndQueuesIt(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1, 2)

	ids := messageIDsOf(t, s, inbox)
	if err := s.ApplyFlagChange(ctx, []int64{ids[1]}, []string{model.FlagSeen}, true); err != nil {
		t.Fatalf("ApplyFlagChange() error: %v", err)
	}

	if got := flagsOf(t, s, inbox, 1); len(got) != 1 {
		t.Errorf("UID 1 flags = %v, want the seen flag applied locally", got)
	}
	if got := flagsOf(t, s, inbox, 2); len(got) != 0 {
		t.Errorf("UID 2 flags = %v, want it untouched", got)
	}

	ops, err := s.ClaimOperations(ctx, acct, 10)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("queued %d operations, want 1", len(ops))
	}
	if ops[0].Kind != model.OpAddFlags || len(ops[0].UIDs) != 1 || ops[0].UIDs[0] != 1 {
		t.Errorf("queued %+v, want an add-flags for UID 1", ops[0])
	}
}

// The stamp has to be the folder's generation at the moment of queueing, or
// the worker's check has nothing meaningful to compare against.
func TestQueuedActionCarriesTheFoldersUIDValidity(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1)

	// Give the folder a generation, the way a sync would.
	if err := s.UpsertFolders(ctx, acct, []model.Folder{
		{Path: "INBOX", Name: "INBOX", UIDValidity: 777},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	// UIDValidity is only written on insert, so reset it the way the engine
	// would when it notices a change.
	if err := s.ResetFolder(ctx, inbox, 777); err != nil {
		t.Fatalf("ResetFolder() error: %v", err)
	}
	seedFolderMessages(t, s, acct, inbox, 1)

	ids := messageIDsOf(t, s, inbox)
	if err := s.ApplyFlagChange(ctx, []int64{ids[1]}, []string{model.FlagSeen}, true); err != nil {
		t.Fatalf("ApplyFlagChange() error: %v", err)
	}

	ops, _ := s.ClaimOperations(ctx, acct, 10)
	if len(ops) != 1 || ops[0].UIDValidity != 777 {
		t.Errorf("queued %+v, want the folder's UIDVALIDITY of 777", ops)
	}
}

// A selection can span folders, and a UID only means something inside one.
// One operation covering both would name UIDs from the wrong mailbox.
func TestApplyFlagChangeSplitsPerFolder(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "Arşiv", Name: "Arşiv"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	archive := folderIDByPath(t, s, acct, "Arşiv")

	seedFolderMessages(t, s, acct, inbox, 1)
	seedFolderMessages(t, s, acct, archive, 1)

	selected := []int64{messageIDsOf(t, s, inbox)[1], messageIDsOf(t, s, archive)[1]}
	if err := s.ApplyFlagChange(ctx, selected, []string{model.FlagSeen}, true); err != nil {
		t.Fatalf("ApplyFlagChange() error: %v", err)
	}

	ops, _ := s.ClaimOperations(ctx, acct, 10)
	if len(ops) != 2 {
		t.Fatalf("queued %d operations, want one per folder", len(ops))
	}
	if ops[0].FolderID == ops[1].FolderID {
		t.Errorf("both operations name folder %d", ops[0].FolderID)
	}
}

// Removing a flag is the same path in the other direction, and the queued
// intent has to say which direction it was.
func TestApplyFlagChangeCanRemove(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, inbox, []model.Message{{
		AccountID: acct, FolderID: inbox, UID: 1,
		Subject: "Okundu", InternalDate: time.Unix(1, 0),
		Flags: []string{model.FlagSeen, model.FlagFlagged},
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	ids := messageIDsOf(t, s, inbox)
	if err := s.ApplyFlagChange(ctx, []int64{ids[1]}, []string{model.FlagSeen}, false); err != nil {
		t.Fatalf("ApplyFlagChange() error: %v", err)
	}

	got := flagsOf(t, s, inbox, 1)
	if len(got) != 1 || got[0] != model.FlagFlagged {
		t.Errorf("flags = %v, want only the flagged one left", got)
	}

	ops, _ := s.ClaimOperations(ctx, acct, 10)
	if len(ops) != 1 || ops[0].Kind != model.OpRemoveFlags {
		t.Errorf("queued %+v, want a remove-flags", ops)
	}
}

// Adding a flag a message already has must not queue anything. The IDLE loop
// and the reading pane both mark messages read, and a queue that grew on every
// redundant call would send thousands of pointless commands.
func TestApplyFlagChangeQueuesNothingWhenAlreadyInThatState(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, inbox, []model.Message{{
		AccountID: acct, FolderID: inbox, UID: 1,
		Subject: "Zaten okundu", InternalDate: time.Unix(1, 0),
		Flags: []string{model.FlagSeen},
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	ids := messageIDsOf(t, s, inbox)
	if err := s.ApplyFlagChange(ctx, []int64{ids[1]}, []string{model.FlagSeen}, true); err != nil {
		t.Fatalf("ApplyFlagChange() error: %v", err)
	}

	if ops, _ := s.ClaimOperations(ctx, acct, 10); len(ops) != 0 {
		t.Errorf("queued %+v for a message already in that state", ops)
	}
}

// The move disappears the message from the folder it left. The server assigns
// a new UID in the destination, so the row cannot simply be repointed — the
// destination's next sync is what brings it back, with the UID it really has.
func TestApplyMoveRemovesLocallyAndQueuesTheMove(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "Arşiv", Name: "Arşiv"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	archive := folderIDByPath(t, s, acct, "Arşiv")
	seedFolderMessages(t, s, acct, inbox, 1, 2)

	ids := messageIDsOf(t, s, inbox)
	if err := applyMoveNow(t, s, []int64{ids[1]}, archive); err != nil {
		t.Fatalf("ApplyMove() error: %v", err)
	}

	left, _ := s.ListMessageUIDs(ctx, inbox)
	if len(left) != 1 || left[0] != 2 {
		t.Errorf("the source folder holds %v, want the moved message gone", left)
	}

	ops, _ := s.ClaimOperations(ctx, acct, 10)
	if len(ops) != 1 || ops[0].Kind != model.OpMove || ops[0].TargetFolderID != archive {
		t.Errorf("queued %+v, want a move to the archive", ops)
	}
}

func TestApplyDeleteRemovesLocallyAndQueuesIt(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1, 2)

	ids := messageIDsOf(t, s, inbox)
	if err := applyDeleteNow(t, s, []int64{ids[1]}); err != nil {
		t.Fatalf("ApplyDelete() error: %v", err)
	}

	left, _ := s.ListMessageUIDs(ctx, inbox)
	if len(left) != 1 || left[0] != 2 {
		t.Errorf("folder holds %v, want UID 1 gone", left)
	}

	ops, _ := s.ClaimOperations(ctx, acct, 10)
	if len(ops) != 1 || ops[0].Kind != model.OpDelete {
		t.Errorf("queued %+v, want a delete", ops)
	}
	assertFTSIntegrity(t, s)
}

// A message the retention window already removed, or one deleted on another
// device a second ago, is not an error. The window just has a stale list.
func TestActionsIgnoreMessagesThatAreNoLongerHere(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1)

	for _, err := range []error{
		s.ApplyFlagChange(ctx, []int64{4242}, []string{model.FlagSeen}, true),
		applyMoveNow(t, s, []int64{4242}, inbox),
		applyDeleteNow(t, s, []int64{4242}),
	} {
		if err != nil {
			t.Errorf("acting on an unknown message returned %v", err)
		}
	}

	if ops, _ := s.ClaimOperations(ctx, acct, 10); len(ops) != 0 {
		t.Errorf("queued %+v for messages that are not here", ops)
	}
}

func TestActionsOnAnEmptySelectionDoNothing(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, _ := seedInbox(t, s)

	if err := s.ApplyFlagChange(ctx, nil, []string{model.FlagSeen}, true); err != nil {
		t.Errorf("ApplyFlagChange(nil) error: %v", err)
	}
	if err := applyDeleteNow(t, s, nil); err != nil {
		t.Errorf("ApplyDelete(nil) error: %v", err)
	}
	if ops, _ := s.ClaimOperations(ctx, acct, 10); len(ops) != 0 {
		t.Errorf("an empty selection queued %+v", ops)
	}
}

// applyMoveNow and applyDeleteNow run an action with no undo window, which is
// what these tests are about: what gets queued, not when it goes out.
func applyMoveNow(t *testing.T, s *Store, ids []int64, target int64) error {
	t.Helper()
	_, err := s.ApplyMove(context.Background(), ids, target, time.Time{})
	return err
}

func applyDeleteNow(t *testing.T, s *Store, ids []int64) error {
	t.Helper()
	_, err := s.ApplyDelete(context.Background(), ids, time.Time{})
	return err
}
