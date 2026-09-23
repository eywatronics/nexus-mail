package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nexusmail/internal/model"
)

// deleteBatchSize bounds how many UIDs go into one DELETE.
//
// SQLite caps the number of bind parameters per statement, and an expunge of a
// large mailbox can name tens of thousands of UIDs. Chunking keeps the
// statement inside the limit; doing it one UID at a time instead is what makes
// a big expunge take minutes.
const deleteBatchSize = 500

// ListMessageUIDs returns the UIDs a folder holds, ascending.
//
// This is one half of detecting a deletion: the caller diffs it against what
// the server still reports. The order is part of the contract — both sides
// sorted means the diff is a single walk rather than a set built in memory for
// a folder with fifty thousand messages.
func (s *Store) ListMessageUIDs(ctx context.Context, folderID int64) ([]uint32, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT uid FROM messages WHERE folder_id = ? ORDER BY uid`, folderID)
	if err != nil {
		return nil, fmt.Errorf("store: listing UIDs for folder %d: %w", folderID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []uint32
	for rows.Next() {
		var uid uint32
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

// SetMessageFlags applies the server's current flags to messages we already
// hold.
//
// Deliberately an UPDATE and not an upsert. A UID the server reports but we
// have no row for is skipped, because that is the normal consequence of
// syncing a window rather than a whole mailbox: the server talks about its
// entire history, we keep the recent part. Inserting a flags-only row would
// create a message with no sender, no subject and no date.
//
// Flags are replaced wholesale rather than merged. IMAP reports the complete
// set for a message, so a merge would keep a flag the server has removed —
// the message someone marked unread on their phone would stay read here.
func (s *Store) SetMessageFlags(ctx context.Context, folderID int64, updates []model.FlagUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin flag update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE messages SET flags = ? WHERE folder_id = ? AND uid = ?`)
	if err != nil {
		return fmt.Errorf("store: prepare flag update: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, u := range updates {
		flags := u.Flags
		if flags == nil {
			// A message with every flag removed is a real state; encoding it
			// as SQL NULL rather than "[]" would make the column ambiguous.
			flags = []string{}
		}
		encoded, err := json.Marshal(flags)
		if err != nil {
			return fmt.Errorf("store: encode flags for UID %d: %w", u.UID, err)
		}
		if _, err := stmt.ExecContext(ctx, string(encoded), folderID, u.UID); err != nil {
			return fmt.Errorf("store: updating flags for UID %d: %w", u.UID, err)
		}
	}

	return tx.Commit()
}

// DeleteMessagesByUID removes messages the server no longer has.
//
// Scoped to one folder because UIDs are unique per folder, not per account.
// The same number identifies a different message in every mailbox, so a
// delete that forgot the folder would silently remove unrelated mail — and it
// would look correct in testing right up until two folders happened to share
// a UID.
//
// The FTS index and the message bodies follow through the schema's triggers
// and cascades; nothing here touches them directly.
func (s *Store) DeleteMessagesByUID(ctx context.Context, folderID int64, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}

	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin message delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for start := 0; start < len(uids); start += deleteBatchSize {
		end := min(start+deleteBatchSize, len(uids))
		batch := uids[start:end]

		args := make([]any, 0, len(batch)+1)
		args = append(args, folderID)
		for _, uid := range batch {
			args = append(args, uid)
		}

		query := `DELETE FROM messages WHERE folder_id = ? AND uid IN (` +
			placeholders(len(batch)) + `)`
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("store: deleting %d messages from folder %d: %w",
				len(batch), folderID, err)
		}
	}

	return tx.Commit()
}

// placeholders builds "?, ?, ?" for an IN clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
