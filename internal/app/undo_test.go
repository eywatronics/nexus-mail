package app

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// undoFixture gives a service with a real undo window, one synced account, a
// trash folder, and the id of the message in the inbox.
func undoFixture(t *testing.T) (*MailService, int64, int64, int64) {
	t.Helper()

	svc, _, db := newTestService(t, stubBackend{})
	svc.cfg.UndoWindow = defaultUndoWindow
	ctx := context.Background()

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}
	if err := db.UpsertFolders(ctx, acct.ID, []model.Folder{
		{Path: "Çöp Kutusu", Name: "Çöp Kutusu"},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}

	folders, _ := svc.ListFolders(acct.ID)
	var inbox, trash int64
	for _, f := range folders {
		if f.IsInbox {
			inbox = f.ID
		}
		if f.Role == "trash" {
			trash = f.ID
		}
	}
	msgs, err := svc.ListMessages(inbox, 10, 0, false)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("ListMessages() = %v, %v", msgs, err)
	}
	return svc, msgs[0].ID, inbox, trash
}

// The message has to come back where it was, and the change has to not reach
// the server. Either half alone is a bug: a restored row whose deletion still
// went out is a message the reader can see and the server has thrown away.
func TestUndoPutsTheMessageBackAndCancelsTheChange(t *testing.T) {
	svc, id, inbox, _ := undoFixture(t)
	ctx := context.Background()

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if got := messageCount(t, svc, inbox); got != 0 {
		t.Fatalf("the inbox still holds %d messages before the undo", got)
	}

	undone, err := svc.UndoLastAction()
	if err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if !undone {
		t.Fatal("UndoLastAction() reported nothing was taken back")
	}

	if got := messageCount(t, svc, inbox); got != 1 {
		t.Errorf("the inbox holds %d messages after the undo, want the message back", got)
	}

	pending, err := svc.store.CountOperations(ctx, 1, model.OpPending)
	if err != nil {
		t.Fatalf("CountOperations() error: %v", err)
	}
	if pending != 0 {
		t.Errorf("%d operations are still queued after the undo", pending)
	}
}

// Undo has to survive the round trip through the database, not just the
// in-memory list: a restored message the window shows but the store does not
// hold would vanish on the next refresh.
func TestTheRestoredMessageIsReallyThere(t *testing.T) {
	svc, id, inbox, _ := undoFixture(t)

	before, err := svc.ListMessages(inbox, 10, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}

	after, err := svc.ListMessages(inbox, 10, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("the folder holds %d messages, want %d", len(after), len(before))
	}
	if after[0].Subject != before[0].Subject || after[0].UID != before[0].UID {
		t.Errorf("the restored message is %+v, want %+v", after[0], before[0])
	}
}

// A move is undone the same way, because a delete is already a move.
func TestUndoTakesBackAMove(t *testing.T) {
	svc, id, inbox, trash := undoFixture(t)

	if err := svc.MoveMessages([]int64{id}, trash); err != nil {
		t.Fatalf("MoveMessages() error: %v", err)
	}
	undone, err := svc.UndoLastAction()
	if err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if !undone {
		t.Fatal("a move was not taken back")
	}
	if got := messageCount(t, svc, inbox); got != 1 {
		t.Errorf("the source folder holds %d messages after the undo", got)
	}
}

// The offer names what it would take back, so it can say "Moved to trash"
// rather than a bare "Undo" the reader has to guess at.
func TestTheOfferSaysWhatItWouldTakeBack(t *testing.T) {
	svc, id, _, _ := undoFixture(t)

	if got := svc.Undoable().Kind; got != UndoNone {
		t.Errorf("Undoable() = %q before anything happened", got)
	}

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	offer := svc.Undoable()
	if offer.Kind != UndoTrash {
		t.Errorf("Kind = %q after deleting, want trash", offer.Kind)
	}
	if offer.Count != 1 {
		t.Errorf("Count = %d, want 1", offer.Count)
	}
	if offer.ExpiresUnixMs <= time.Now().UnixMilli() {
		t.Error("the offer has already expired")
	}

	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if got := svc.Undoable().Kind; got != UndoNone {
		t.Errorf("Undoable() = %q after the undo was taken", got)
	}
}

// Once the change has gone out there is nothing to cancel, and saying
// otherwise would send the reader looking for a message that is not there.
func TestUndoIsRefusedOnceTheChangeIsOnItsWay(t *testing.T) {
	svc, id, inbox, _ := undoFixture(t)
	ctx := context.Background()

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}

	ops, err := svc.store.ClaimOperations(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	if len(ops) != 0 {
		t.Fatal("the operation was claimable before its window closed")
	}

	// Stand in for the window having closed and the worker having taken it.
	if _, err := svc.store.Write().ExecContext(ctx,
		"UPDATE operations SET next_attempt_at = 0"); err != nil {
		t.Fatalf("making the operation due: %v", err)
	}

	undone, err := svc.UndoLastAction()
	if err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if undone {
		t.Error("UndoLastAction() claimed to take back a change already on its way")
	}
	if got := messageCount(t, svc, inbox); got != 0 {
		t.Error("the message came back although the change had gone out")
	}
}

// With the window off there is nothing to offer, and offering it anyway would
// be a button that always fails.
func TestNoWindowMeansNoUndo(t *testing.T) {
	svc, id, _, _ := undoFixture(t)
	svc.cfg.UndoWindow = 0

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if got := svc.Undoable().Kind; got != UndoNone {
		t.Errorf("Undoable() = %q with the window off", got)
	}

	undone, err := svc.UndoLastAction()
	if err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if undone {
		t.Error("something was taken back with the window off")
	}

	// And the change is claimable at once, which is the point of turning the
	// window off.
	ops, _ := svc.store.ClaimOperations(context.Background(), 1, 10)
	if len(ops) != 1 {
		t.Errorf("claimed %d operations with the window off, want 1", len(ops))
	}
}

func TestUndoWithNothingToUndoIsNotAnError(t *testing.T) {
	svc, _, _, _ := undoFixture(t)

	undone, err := svc.UndoLastAction()
	if err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if undone {
		t.Error("UndoLastAction() took something back out of nothing")
	}
}

// One action deep. An older entry the server has already caught up with would
// make undo a button that silently did nothing.
func TestOnlyTheLastActionIsHeld(t *testing.T) {
	svc, id, inbox, trash := undoFixture(t)
	ctx := context.Background()

	if err := svc.MoveMessages([]int64{id}, trash); err != nil {
		t.Fatalf("MoveMessages() error: %v", err)
	}

	// A second action replaces the first.
	if err := svc.store.UpsertMessages(ctx, inbox, []model.Message{
		{AccountID: 1, FolderID: inbox, UID: 900, Subject: "İkinci"},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	msgs, _ := svc.ListMessages(inbox, 10, 0, false)
	if err := svc.DeleteMessages([]int64{msgs[0].ID}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}

	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}

	// The second action came back; the first is still on its way out.
	if got := messageCount(t, svc, inbox); got != 1 {
		t.Errorf("the inbox holds %d messages, want only the second action undone", got)
	}
	if got := svc.Undoable().Kind; got != UndoNone {
		t.Errorf("Undoable() = %q; the first action must not resurface", got)
	}
}
