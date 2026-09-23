package app

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// The obvious one: undo put it back, redo takes it away again.
func TestRedoDoesTheActionAgain(t *testing.T) {
	svc, id, inbox, _ := undoFixture(t)

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if got := messageCount(t, svc, inbox); got != 1 {
		t.Fatalf("the inbox holds %d messages after the undo, want 1", got)
	}

	redone, err := svc.RedoLastAction()
	if err != nil {
		t.Fatalf("RedoLastAction() error: %v", err)
	}
	if !redone {
		t.Fatal("RedoLastAction() reported it did nothing")
	}
	if got := messageCount(t, svc, inbox); got != 0 {
		t.Errorf("the inbox holds %d messages after the redo, want 0", got)
	}
}

// A restored message is usually not the row it was, because the id column
// hands out max(rowid)+1. A redo that carried the old id across the undo would
// act on nothing — or, once that number is handed out again, on the wrong
// message.
func TestRedoFindsTheMessageUnderItsNewID(t *testing.T) {
	svc, first, inbox, _ := undoFixture(t)
	ctx := context.Background()

	// A second, newer message, so the one being deleted is not the highest row
	// in the table and cannot get its number back.
	if err := svc.store.UpsertMessages(ctx, inbox, []model.Message{
		{AccountID: 1, FolderID: inbox, UID: 900, Subject: "Sonraki"},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	if err := svc.DeleteMessages([]int64{first}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}

	// The premise: the restored row really did change id. Without this the
	// test would pass for the wrong reason.
	msgs, _ := svc.ListMessages(inbox, 10, 0, false)
	var restoredID int64
	for _, m := range msgs {
		if m.UID == 1 {
			restoredID = m.ID
		}
	}
	if restoredID == 0 {
		t.Fatalf("the restored message is not in the folder: %+v", msgs)
	}
	if restoredID == first {
		t.Skip("the row kept its id here, so this test cannot say anything")
	}

	if _, err := svc.RedoLastAction(); err != nil {
		t.Fatalf("RedoLastAction() error: %v", err)
	}
	if got := messageCount(t, svc, inbox); got != 1 {
		t.Errorf("the inbox holds %d messages, want only the second one", got)
	}
}

// A redone move has to go back to the folder it was going to, which is the
// only thing about a redo a delete cannot work out for itself.
func TestRedoSendsAMoveBackToTheSameFolder(t *testing.T) {
	svc, id, inbox, trash := undoFixture(t)

	if err := svc.MoveMessages([]int64{id}, trash); err != nil {
		t.Fatalf("MoveMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if _, err := svc.RedoLastAction(); err != nil {
		t.Fatalf("RedoLastAction() error: %v", err)
	}

	if got := messageCount(t, svc, inbox); got != 0 {
		t.Errorf("the message is still in the source folder after the redo")
	}
	if got := messageCount(t, svc, trash); got != 0 {
		// It is queued for the target rather than shown there: a local row in
		// the destination would need a UID the server has not assigned yet.
		t.Errorf("the message was put in the target folder locally")
	}
}

// A redone action is an ordinary action, so it is takeable back in its turn.
// Anything else would make the second press of undo the one that does nothing.
func TestARedoneActionCanBeUndoneAgain(t *testing.T) {
	svc, id, inbox, _ := undoFixture(t)

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if _, err := svc.RedoLastAction(); err != nil {
		t.Fatalf("RedoLastAction() error: %v", err)
	}

	if got := svc.Undoable().Kind; got != UndoTrash {
		t.Errorf("Undoable() = %q after a redo, want the redone action", got)
	}
	undone, err := svc.UndoLastAction()
	if err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if !undone {
		t.Fatal("the redone action could not be taken back")
	}
	if got := messageCount(t, svc, inbox); got != 1 {
		t.Errorf("the inbox holds %d messages, want the message back again", got)
	}
}

// Nothing was undone, so there is nothing to do again.
func TestThereIsNothingToRedoBeforeAnUndo(t *testing.T) {
	svc, id, _, _ := undoFixture(t)

	if got := svc.Redoable().Kind; got != UndoNone {
		t.Errorf("Redoable() = %q before anything happened", got)
	}

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if got := svc.Redoable().Kind; got != UndoNone {
		t.Errorf("Redoable() = %q after an action that was not undone", got)
	}

	redone, err := svc.RedoLastAction()
	if err != nil {
		t.Fatalf("RedoLastAction() error: %v", err)
	}
	if redone {
		t.Error("RedoLastAction() did something out of nothing")
	}
}

// The offer names the action, so the notice can say what pressing it would do
// rather than a bare "Redo" the reader has to guess at.
func TestTheRedoOfferNamesTheAction(t *testing.T) {
	svc, id, _, trash := undoFixture(t)

	if err := svc.MoveMessages([]int64{id}, trash); err != nil {
		t.Fatalf("MoveMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}

	offer := svc.Redoable()
	if offer.Kind != UndoMove {
		t.Errorf("Kind = %q, want move", offer.Kind)
	}
	if offer.Count != 1 {
		t.Errorf("Count = %d, want 1", offer.Count)
	}
	if offer.ExpiresUnixMs <= time.Now().UnixMilli() {
		t.Error("the redo offer has already expired")
	}
}

// One step, like the undo. A redo that reached back past the action in between
// would be a history, which this deliberately is not.
func TestANewActionEndsTheRedo(t *testing.T) {
	svc, id, inbox, _ := undoFixture(t)
	ctx := context.Background()

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}

	if err := svc.store.UpsertMessages(ctx, inbox, []model.Message{
		{AccountID: 1, FolderID: inbox, UID: 900, Subject: "Baska"},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	msgs, _ := svc.ListMessages(inbox, 10, 0, false)
	if err := svc.DeleteMessages([]int64{msgs[0].ID}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}

	if got := svc.Redoable().Kind; got != UndoNone {
		t.Errorf("Redoable() = %q after a new action", got)
	}
	redone, _ := svc.RedoLastAction()
	if redone {
		t.Error("a redo survived the action that replaced it")
	}
}

// Emptying the trash may have destroyed the very messages the redo names, and
// doing the action again to rows whose server side is gone is not something to
// leave a keystroke away.
func TestEmptyingTheTrashEndsTheRedoToo(t *testing.T) {
	svc, id, _, _ := undoFixture(t)

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}
	if got := svc.Redoable().Kind; got == UndoNone {
		t.Fatal("there was no redo to lose")
	}

	if err := svc.EmptyTrash(1); err != nil {
		t.Fatalf("EmptyTrash() error: %v", err)
	}
	if got := svc.Redoable().Kind; got != UndoNone {
		t.Errorf("Redoable() = %q after the trash was emptied", got)
	}
}

// The redo expires on the same window an undo does. An undo and a redo are
// both a moment of hesitation, and one still live much later would be a
// keystroke that silently deletes mail the reader had decided to keep.
func TestTheRedoOfferExpires(t *testing.T) {
	svc, id, inbox, _ := undoFixture(t)
	svc.liveUndoWindow = 40 * time.Millisecond

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}
	if _, err := svc.UndoLastAction(); err != nil {
		t.Fatalf("UndoLastAction() error: %v", err)
	}

	time.Sleep(80 * time.Millisecond)

	if got := svc.Redoable().Kind; got != UndoNone {
		t.Errorf("Redoable() = %q past the window", got)
	}
	redone, err := svc.RedoLastAction()
	if err != nil {
		t.Fatalf("RedoLastAction() error: %v", err)
	}
	if redone {
		t.Error("an expired redo was taken")
	}
	if got := messageCount(t, svc, inbox); got != 1 {
		t.Error("the message was deleted again by an expired redo")
	}
}
