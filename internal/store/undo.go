package store

import (
	"context"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

// MessagesByIDs reads whole messages, for the callers that need to put them
// back.
//
// Whole rows rather than the handful of columns an action needs, because the
// purpose is a snapshot: what goes into it has to be enough to restore the
// message exactly, and a snapshot that quietly dropped a column would restore
// a message missing something nobody noticed until they looked for it.
func (s *Store) MessagesByIDs(ctx context.Context, messageIDs []int64) ([]model.Message, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}

	args := make([]any, 0, len(messageIDs))
	for _, id := range messageIDs {
		args = append(args, id)
	}

	rows, err := s.read.QueryContext(ctx,
		`SELECT `+messageColumns+`
		 FROM messages WHERE id IN (`+placeholders(len(messageIDs))+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: reading messages to snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanMessageRows(rows)
}

// CancelOperations removes queued operations that have not started going out,
// and reports how many it removed.
//
// The condition is the whole design. An operation is claimable once its
// next_attempt_at has passed, so requiring it to still be in the future means
// the worker cannot have read it: the two windows are disjoint by
// construction, because time only moves forward. A cancel that merely checked
// "still pending" would race a worker mid-send and leave the local state
// claiming a change was taken back that the server had already applied.
//
// Removing the row rather than marking it dropped: dropped is the word for a
// change the client gave up on, which the user is then told about. A change
// the user cancelled themselves is not news.
func (s *Store) CancelOperations(ctx context.Context, ids []int64, now time.Time) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	args := make([]any, 0, len(ids)+2)
	args = append(args, string(model.OpPending), now.UnixNano())
	for _, id := range ids {
		args = append(args, id)
	}

	res, err := s.write.ExecContext(ctx,
		`DELETE FROM operations
		  WHERE state = ? AND next_attempt_at > ?
		    AND id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return 0, fmt.Errorf("store: cancelling queued operations: %w", err)
	}
	affected, err := res.RowsAffected()
	return int(affected), err
}

// RestoreMessages puts snapshotted messages back where they were.
//
// Safe only for a message whose removal never reached the server, which is
// what CancelOperations establishes: the UID in the snapshot still names the
// same message in the same folder, because nothing moved it.
func (s *Store) RestoreMessages(ctx context.Context, msgs []model.Message) error {
	byFolder := map[int64][]model.Message{}
	for _, m := range msgs {
		byFolder[m.FolderID] = append(byFolder[m.FolderID], m)
	}
	for folderID, batch := range byFolder {
		if err := s.UpsertMessages(ctx, folderID, batch); err != nil {
			return err
		}
	}
	return nil
}
