package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexusmail/internal/model"
)

// MessagesAfterID returns messages in a folder whose row id is above a
// watermark, oldest first.
//
// The row id rather than a date, because it is the only thing here that is
// monotonic in the order this client learned about messages. A date would
// announce a message that arrived last week but only reached us today as
// something new — which, from the reader's point of view, it is not, and from
// the notification's point of view is the difference between a useful alert
// and an alert about a mailbox being backfilled.
//
// Capped, because the caller is deciding what to put in a notification and a
// thousand arrivals produce one sentence, not a thousand.
func (s *Store) MessagesAfterID(ctx context.Context, folderID, afterID int64, limit int) ([]model.Message, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT `+messageColumns+`
		 FROM messages
		 WHERE folder_id = ? AND id > ?
		 ORDER BY id
		 LIMIT ?`, folderID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: reading arrivals in folder %d: %w", folderID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanMessageRows(rows)
}

// HighestMessageID reports the largest row id in a folder, or zero when it is
// empty.
//
// This is what a watermark starts at. Without it the first pass after startup
// would announce every unread message the mailbox already held, which is the
// one thing a new-mail notification must never do.
func (s *Store) HighestMessageID(ctx context.Context, folderID int64) (int64, error) {
	var highest sql.NullInt64
	err := s.read.QueryRowContext(ctx,
		`SELECT MAX(id) FROM messages WHERE folder_id = ?`, folderID).Scan(&highest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: reading the highest message id in folder %d: %w", folderID, err)
	}
	return highest.Int64, nil
}
