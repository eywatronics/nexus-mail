package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SetMessageBody stores a fetched body and marks the message as having one.
//
// Bodies live in their own table so listing a mailbox never reads them: a
// message list query touches only fixed-width header columns, which is what
// keeps scrolling a large folder cheap.
//
// The body is not indexed for search here. Bodies arrive lazily, so a P0 body
// index would be inherently incomplete; P3 builds one with a single backfill
// over this table.
func (s *Store) SetMessageBody(ctx context.Context, messageID int64, html, text string) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin body write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO message_bodies (message_id, html_body, text_body)
		 VALUES (?, ?, ?)
		 ON CONFLICT(message_id) DO UPDATE SET
		   html_body = excluded.html_body,
		   text_body = excluded.text_body`,
		messageID, html, text); err != nil {
		return fmt.Errorf("store: write body for message %d: %w", messageID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE messages SET body_fetched = 1 WHERE id = ?`, messageID); err != nil {
		return fmt.Errorf("store: mark body fetched for message %d: %w", messageID, err)
	}
	return tx.Commit()
}

// GetMessageBody returns a cached body. A message with none returns empty
// strings and no error: that is the normal state for every message until it is
// first opened, and making it an error would put a branch at every call site.
func (s *Store) GetMessageBody(ctx context.Context, messageID int64) (html, text string, err error) {
	row := s.read.QueryRowContext(ctx,
		`SELECT html_body, text_body FROM message_bodies WHERE message_id = ?`, messageID)

	err = row.Scan(&html, &text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("store: read body for message %d: %w", messageID, err)
	}
	return html, text, nil
}
