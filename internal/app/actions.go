package app

import (
	"context"

	"nexusmail/internal/model"
)

// MarkRead marks messages read or unread.
//
// The local database is updated and the same change is queued for the server,
// in one transaction. The window redraws from the local state immediately —
// that is the point of the architecture, and it is also why the queue exists:
// without it the window and the server would disagree the moment the network
// hiccuped.
func (s *MailService) MarkRead(messageIDs []int64, read bool) error {
	return s.applyFlags(messageIDs, []string{model.FlagSeen}, read)
}

// SetStarred stars or unstars messages.
func (s *MailService) SetStarred(messageIDs []int64, starred bool) error {
	return s.applyFlags(messageIDs, []string{model.FlagFlagged}, starred)
}

func (s *MailService) applyFlags(messageIDs []int64, flags []string, add bool) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := s.store.ApplyFlagChange(context.Background(), messageIDs, flags, add); err != nil {
		return err
	}
	s.nudgeWatchers()
	return nil
}

// MoveMessages moves messages to another folder.
//
// They leave the current folder immediately and reappear in the destination
// after its next sync, with the UID the server actually assigned. Showing them
// in the destination straight away would mean inventing a UID, and a row
// carrying a made-up one is wrong in a way no later sync can repair.
func (s *MailService) MoveMessages(messageIDs []int64, targetFolderID int64) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := s.store.ApplyMove(context.Background(), messageIDs, targetFolderID); err != nil {
		return err
	}
	s.nudgeWatchers()
	return nil
}

// DeleteMessages deletes messages.
func (s *MailService) DeleteMessages(messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := s.store.ApplyDelete(context.Background(), messageIDs); err != nil {
		return err
	}
	s.nudgeWatchers()
	return nil
}

// PendingChangeCount reports how many changes are still waiting to reach the
// server, and how many were dropped because the mailbox they belonged to was
// recreated.
//
// Dropped changes are the honest half of the UIDVALIDITY rule: rather than
// applying an operation to whatever message now carries that number, the
// worker gives up on it. That is a loss of the user's intent, and they are
// owed the news.
func (s *MailService) PendingChangeCount(accountID int64) (PendingChangesDTO, error) {
	ctx := context.Background()

	pending, err := s.store.CountOperations(ctx, accountID, model.OpPending)
	if err != nil {
		return PendingChangesDTO{}, err
	}
	dropped, err := s.store.CountOperations(ctx, accountID, model.OpDropped)
	if err != nil {
		return PendingChangesDTO{}, err
	}
	failed, err := s.store.CountOperations(ctx, accountID, model.OpFailed)
	if err != nil {
		return PendingChangesDTO{}, err
	}
	return PendingChangesDTO{Pending: pending, Dropped: dropped, Failed: failed}, nil
}

// nudgeWatchers asks every running watch loop to drain now rather than waiting
// for the server to happen to say something.
//
// Every watched account is nudged, not only the one the messages belong to.
// Working out which account a selection touched would mean a second query, and
// a nudge to an account with an empty queue costs one database read.
func (s *MailService) nudgeWatchers() {
	s.watchMu.Lock()
	ids := make([]int64, 0, len(s.watching))
	for id := range s.watching {
		ids = append(ids, id)
	}
	s.watchMu.Unlock()

	for _, id := range ids {
		s.engine.Nudge(id)
	}
}

// AcknowledgeChangeFailures clears the record of changes that were dropped or
// permanently failed, so the notice about them goes away.
func (s *MailService) AcknowledgeChangeFailures(accountID int64) error {
	return s.store.ForgetFinishedOperations(context.Background(), accountID)
}
