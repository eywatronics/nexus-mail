package app

import (
	"context"
	"fmt"
)

// EmptyTrash destroys everything in an account's trash, on the server as well
// as here.
//
// Not expressed as a delete of the messages we hold. The local rows are only
// the part of the folder this client downloaded — the retention window caps
// what is kept, and a folder nobody has opened has nothing at all — so a
// version built from our own list would report success on a trash it had
// barely touched. The queued operation says "empty this mailbox" and lets the
// server decide what everything means.
//
// Deliberately not undoable. The undo window works by holding the change and
// restoring a snapshot, and a snapshot of a folder this client has not fully
// synced would restore a fraction of what was there — an undo that half worked
// and said nothing about the half it lost. Emptying the trash asks first
// instead, which is the honest trade: a question before, rather than a promise
// afterwards that cannot be kept.
func (s *MailService) EmptyTrash(accountID int64) error {
	ctx := context.Background()

	trashID := s.trashFolderID(ctx, accountID)
	if trashID == 0 {
		return fmt.Errorf("app: account %d has no trash folder", accountID)
	}

	if _, err := s.store.ApplyEmptyFolder(ctx, trashID); err != nil {
		return err
	}

	// The window redraws from the same event a sync pass fires, so it does not
	// need a second way of hearing that a folder changed.
	s.cfg.Emit(EventSyncFinished, SyncEvent{AccountID: accountID})

	// The last action is no longer takeable back: its messages may have been
	// among the ones just destroyed, and restoring them would put back rows
	// whose server side is gone.
	s.forgetUndo()

	s.nudgeWatchers()
	return nil
}

// forgetUndo drops the held action, and the redo with it, without cancelling
// either.
//
// The redo goes for the same reason the undo does: it names messages that may
// have been among the ones just destroyed, and doing the action again to rows
// whose server side is gone is not something to leave a keystroke away.
func (s *MailService) forgetUndo() {
	s.watchMu.Lock()
	s.undo = nil
	s.redo = nil
	s.watchMu.Unlock()
}
