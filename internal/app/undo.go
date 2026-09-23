package app

import (
	"context"
	"time"

	"nexusmail/internal/model"
)

// defaultUndoWindow is how long a destructive change waits before it goes out.
//
// Five seconds is the number the rest of the world settled on for "undo send",
// and the reasoning carries: long enough to notice a mistake and reach for
// Ctrl+Z, short enough that somebody checking the same mailbox on a phone will
// not see the two disagree.
//
// The cost is honest and small: a delete takes five seconds to reach the
// server. The alternative — sending immediately and undoing afterwards —
// means finding the message again in the folder it was moved to, under a UID
// the server assigned and we have not synced yet, which is slower, less
// reliable, and impossible while offline.
const defaultUndoWindow = 5 * time.Second

// undoWindow is the window in force. Zero means a change goes out at once and
// nothing is offered to take back, which is what every mail client did before
// this one.
//
// Read from the live setting rather than from cfg, because the settings screen
// can change it while watch goroutines are running.
func (s *MailService) undoWindow() time.Duration {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.liveUndoWindow
}

// notificationPreview reports whether a new-mail notification may carry the
// sender and subject.
func (s *MailService) notificationPreview() bool {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.liveNotificationPreview
}

// UndoableKind says what the last action was, so the window can name it.
type UndoableKind string

const (
	UndoNone   UndoableKind = ""
	UndoMove   UndoableKind = "move"
	UndoDelete UndoableKind = "delete"
	UndoTrash  UndoableKind = "trash"
)

// UndoableDTO describes what pressing undo would take back.
type UndoableDTO struct {
	Kind UndoableKind `json:"kind"`
	// Count is how many messages the action covered.
	Count int `json:"count"`
	// ExpiresUnixMs is when the action goes out and stops being undoable, so
	// the window can hide the offer rather than leave a button that fails.
	ExpiresUnixMs int64 `json:"expiresUnixMs"`
}

// undoable is one action held open, with everything needed to put it back.
type undoable struct {
	kind         UndoableKind
	operationIDs []int64
	// snapshot is the messages as they were, whole. Restoring is an upsert of
	// exactly what was there, which is only correct because the operation
	// never reached the server — see Store.CancelOperations.
	snapshot []model.Message
	// target is the folder a move was headed for, so the same move can be made
	// again. Zero for a delete, which works its destination out from the folder
	// roles each time and must keep doing so: the account may have grown a
	// trash folder since.
	target  int64
	expires time.Time
}

// redoable is the action an undo took back, ready to be done again.
type redoable struct {
	kind UndoableKind
	// messageIDs are the rows as they are now, read back after the restore
	// rather than carried across it. A restored message is usually not the row
	// it was — see Store.RestoreMessages.
	messageIDs []int64
	target     int64
	expires    time.Time
}

// rememberUndo holds an action open for the undo window.
//
// One action deep. A stack would have to survive the server catching up with
// the older entries, and an "undo" that silently did nothing because the
// change it referred to had already gone out is worse than no undo: the reader
// believes the message came back.
func (s *MailService) rememberUndo(kind UndoableKind, ops []int64, target int64,
	snapshot []model.Message) {

	if len(ops) == 0 || s.undoWindow() <= 0 {
		return
	}
	s.watchMu.Lock()
	s.undo = &undoable{
		kind: kind, operationIDs: ops, target: target, snapshot: snapshot,
		expires: time.Now().Add(s.undoWindow()),
	}
	// A new action ends the last one's redo. Keeping it would mean a redo that
	// referred to something two steps back, which is a history — and a history
	// is the thing this deliberately is not.
	s.redo = nil
	s.watchMu.Unlock()
}

// Undoable reports what the last action was, or a zero kind when there is
// nothing to take back.
func (s *MailService) Undoable() UndoableDTO {
	s.watchMu.Lock()
	held := s.undo
	s.watchMu.Unlock()

	if held == nil || time.Now().After(held.expires) {
		return UndoableDTO{}
	}
	return UndoableDTO{
		Kind:          held.kind,
		Count:         len(held.snapshot),
		ExpiresUnixMs: held.expires.UnixMilli(),
	}
}

// UndoLastAction takes back the last destructive change, if it has not gone
// out yet.
//
// Reports whether it did. A caller that assumed success would tell the reader
// their mail came back when the queue had already sent the change a moment
// earlier — and the reader would go looking for a message that is not there.
func (s *MailService) UndoLastAction() (bool, error) {
	ctx := context.Background()

	s.watchMu.Lock()
	held := s.undo
	s.undo = nil
	s.watchMu.Unlock()

	if held == nil {
		return false, nil
	}

	cancelled, err := s.store.CancelOperations(ctx, held.operationIDs, time.Now())
	if err != nil {
		return false, err
	}
	// Partially cancelled means some of it is already on its way. Restoring
	// the rest would leave the reader with a folder that agrees with neither
	// what they asked for nor what the server has, so the honest answer is
	// that the undo did not happen.
	if cancelled != len(held.operationIDs) {
		return false, nil
	}

	restored, err := s.store.RestoreMessages(ctx, held.snapshot)
	if err != nil {
		return false, err
	}

	s.watchMu.Lock()
	s.redo = &redoable{
		kind: held.kind, messageIDs: restored, target: held.target,
		expires: time.Now().Add(s.undoWindow()),
	}
	s.watchMu.Unlock()

	s.cfg.Emit(EventSyncFinished, SyncEvent{})
	return true, nil
}

// Redoable reports what pressing redo would do again.
func (s *MailService) Redoable() UndoableDTO {
	s.watchMu.Lock()
	held := s.redo
	s.watchMu.Unlock()

	if held == nil || time.Now().After(held.expires) {
		return UndoableDTO{}
	}
	return UndoableDTO{
		Kind:          held.kind,
		Count:         len(held.messageIDs),
		ExpiresUnixMs: held.expires.UnixMilli(),
	}
}

// RedoLastAction does again what the last undo took back.
//
// It runs the ordinary action rather than replaying anything: a redone delete
// goes through DeleteMessages and is therefore undoable in its turn, gets its
// own queue entry, and works out for itself whether the account has a trash
// folder now. Replaying the cancelled operation would have skipped all three.
//
// The offer expires on the same window an undo does, and that is a decision
// rather than a mechanism. Undo has a deadline because the queue takes the
// change at the end of it; redo has none of its own. But an undo and a redo
// are both a moment of hesitation, and a redo still live ten minutes later
// would be a keystroke that silently deletes mail the reader had long since
// decided to keep.
func (s *MailService) RedoLastAction() (bool, error) {
	s.watchMu.Lock()
	held := s.redo
	s.redo = nil
	s.watchMu.Unlock()

	if held == nil || time.Now().After(held.expires) {
		return false, nil
	}

	var err error
	switch held.kind {
	case UndoMove:
		err = s.MoveMessages(held.messageIDs, held.target)
	case UndoTrash, UndoDelete:
		err = s.DeleteMessages(held.messageIDs)
	default:
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// The same event the undo fires, for the same reason: the window took the
	// rows out of its list when the reader deleted them, but the undo put them
	// back, and nothing else would tell it they have gone again.
	s.cfg.Emit(EventSyncFinished, SyncEvent{})
	return true, nil
}

// scheduleQueueNudge wakes the queue once the undo window has closed.
//
// Without it a change would sit until the next poll — up to a couple of
// minutes — because the watch loop skips operations whose time has not
// arrived and has no reason to come back sooner.
func (s *MailService) scheduleQueueNudge() {
	if s.undoWindow() <= 0 {
		return
	}
	time.AfterFunc(s.undoWindow()+time.Second, s.nudgeWatchers)
}
