package app

import (
	"context"
	"testing"

	"nexusmail/internal/model"
)

// The whole point of the operation. A trash of eight thousand messages with a
// hundred synced must not be "emptied" by destroying the hundred — the queued
// change has to say "everything here", and only the server knows what that is.
func TestEmptyingTheTrashDoesNotNameTheMessagesItKnows(t *testing.T) {
	svc, _, _, trash := undoFixture(t)
	ctx := context.Background()

	// Two messages synced, standing in for a folder that holds far more.
	if err := svc.store.UpsertMessages(ctx, trash, []model.Message{
		{AccountID: 1, FolderID: trash, UID: 11, Subject: "Bir"},
		{AccountID: 1, FolderID: trash, UID: 12, Subject: "İki"},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	if err := svc.EmptyTrash(1); err != nil {
		t.Fatalf("EmptyTrash() error: %v", err)
	}

	ops, err := svc.store.ClaimOperations(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("queued %d operations, want 1", len(ops))
	}
	if ops[0].Kind != model.OpEmptyFolder {
		t.Errorf("queued a %s, want an empty-folder", ops[0].Kind)
	}
	if len(ops[0].UIDs) != 0 {
		t.Errorf("the queued empty names %v; naming UIDs would empty only what was synced", ops[0].UIDs)
	}
	if ops[0].FolderID != trash {
		t.Errorf("the empty names folder %d, want the trash %d", ops[0].FolderID, trash)
	}
}

// The local rows go too, or the window keeps listing mail that is on its way
// to being destroyed.
func TestEmptyingTheTrashClearsItLocally(t *testing.T) {
	svc, _, inbox, trash := undoFixture(t)
	ctx := context.Background()

	if err := svc.store.UpsertMessages(ctx, trash, []model.Message{
		{AccountID: 1, FolderID: trash, UID: 11, Subject: "Bir"},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	if err := svc.EmptyTrash(1); err != nil {
		t.Fatalf("EmptyTrash() error: %v", err)
	}

	if got := messageCount(t, svc, trash); got != 0 {
		t.Errorf("the trash still holds %d messages", got)
	}
	// And nothing else was touched.
	if got := messageCount(t, svc, inbox); got != 1 {
		t.Errorf("the inbox holds %d messages after emptying the trash", got)
	}
}

// An undo held from before the emptying refers to messages that may have been
// among the ones destroyed. Restoring them would put back rows whose server
// side is gone.
func TestEmptyingTheTrashCancelsTheStandingUndo(t *testing.T) {
	svc, id, _, _ := undoFixture(t)

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if svc.Undoable().Kind == UndoNone {
		t.Fatal("precondition: nothing was held to undo")
	}

	if err := svc.EmptyTrash(1); err != nil {
		t.Fatalf("EmptyTrash() error: %v", err)
	}

	if got := svc.Undoable().Kind; got != UndoNone {
		t.Errorf("Undoable() = %q after the trash was emptied", got)
	}
	undone, err := svc.UndoLastAction()
	if err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if undone {
		t.Error("an action from before the emptying was taken back")
	}
}

// An account with no trash cannot be told to empty one, and saying so beats
// quietly doing nothing.
func TestEmptyingWithNoTrashFolderIsRefused(t *testing.T) {
	svc, _, _, _ := trashFixture(t, "")

	if err := svc.EmptyTrash(1); err == nil {
		t.Error("EmptyTrash() succeeded on an account with no trash folder")
	}
}
