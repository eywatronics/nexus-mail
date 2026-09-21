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
// DeleteMessages moves messages to the account's trash, or destroys them if
// they are already there.
//
// Delete used to mean UID EXPUNGE: the message was gone from the server, with
// no trash to find it in and no way back. That is not what Delete means in any
// mail client anybody has used, and it is not a mistake a person gets to make
// twice — the message they deleted by pressing the wrong key is simply gone.
//
// So the ordinary path is a move. The trash folder is found by role, which is
// the server's own special-use attribute where there is one and the folder
// name only as a fallback; a Turkish account's "Çöp Kutusu" is as much the
// trash as "Trash" is.
//
// Deleting from the trash does destroy, because otherwise the trash could
// never be emptied and "delete" there would do nothing at all.
//
// An account with no trash folder falls back to the old behaviour. There is
// nowhere to move the message to, and refusing to delete would be a client
// that cannot delete mail.
func (s *MailService) DeleteMessages(messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}
	ctx := context.Background()

	toTrash, permanent, err := s.splitByDestination(ctx, messageIDs)
	if err != nil {
		return err
	}

	for trashID, ids := range toTrash {
		if err := s.store.ApplyMove(ctx, ids, trashID); err != nil {
			return err
		}
	}
	if len(permanent) > 0 {
		if err := s.store.ApplyDelete(ctx, permanent); err != nil {
			return err
		}
	}

	s.nudgeWatchers()
	return nil
}

// splitByDestination sorts a selection into "move to this trash folder" and
// "destroy".
//
// A selection can span accounts, and each account has its own trash — or none.
// Grouping by destination folder rather than by account is what lets one call
// handle a selection made across a unified view.
func (s *MailService) splitByDestination(ctx context.Context, messageIDs []int64) (
	toTrash map[int64][]int64, permanent []int64, err error) {

	locations, err := s.store.FoldersOfMessages(ctx, messageIDs)
	if err != nil {
		return nil, nil, err
	}

	toTrash = map[int64][]int64{}
	trashCache := map[int64]int64{}

	for _, id := range messageIDs {
		folder, known := locations[id]
		if !known {
			// Gone already: the retention window removed it, or another device
			// did. Nothing to delete and nothing to report.
			continue
		}
		if folder.Role() == model.RoleTrash {
			permanent = append(permanent, id)
			continue
		}

		trashID, looked := trashCache[folder.AccountID]
		if !looked {
			trashID = s.trashFolderID(ctx, folder.AccountID)
			trashCache[folder.AccountID] = trashID
		}
		if trashID == 0 {
			permanent = append(permanent, id)
			continue
		}
		toTrash[trashID] = append(toTrash[trashID], id)
	}
	return toTrash, permanent, nil
}

// trashFolderID finds an account's trash, or zero when it has none.
func (s *MailService) trashFolderID(ctx context.Context, accountID int64) int64 {
	folders, err := s.store.ListFolders(ctx, accountID)
	if err != nil {
		return 0
	}
	for _, f := range folders {
		if f.Role() == model.RoleTrash {
			return f.ID
		}
	}
	return 0
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
