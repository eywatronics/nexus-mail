package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

// defaultDelimiter stands in when a server reports a NIL hierarchy delimiter,
// which happens for flat mailbox layouts. The UI still needs something to
// split paths on.
const defaultDelimiter = "/"

// UpsertFolders writes the folder list for an account. Server-reported counts
// overwrite local ones.
//
// UIDVALIDITY handling is deliberately NOT done here: detecting a change and
// discarding the folder's messages is the sync engine's decision, taken
// through ResetFolder. Doing it implicitly on every folder write would make an
// ordinary refresh capable of wiping a mailbox.
func (s *Store) UpsertFolders(ctx context.Context, accountID int64, folders []model.Folder) error {
	if len(folders) == 0 {
		return nil
	}

	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin folder upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO folders
		   (account_id, name, path, delimiter, attributes, uid_validity, uid_next,
		    highest_modseq, total_count, unread_count, last_synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, path) DO UPDATE SET
		   name           = excluded.name,
		   delimiter      = excluded.delimiter,
		   attributes     = excluded.attributes,
		   uid_validity   = excluded.uid_validity,
		   uid_next       = excluded.uid_next,
		   highest_modseq = excluded.highest_modseq,
		   total_count    = excluded.total_count,
		   unread_count   = excluded.unread_count,
		   last_synced_at = excluded.last_synced_at`)
	if err != nil {
		return fmt.Errorf("store: prepare folder upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, f := range folders {
		attrs, err := json.Marshal(f.Attributes)
		if err != nil {
			return fmt.Errorf("store: encode attributes for %q: %w", f.Path, err)
		}
		delim := f.Delimiter
		if delim == "" {
			delim = defaultDelimiter
		}
		if _, err := stmt.ExecContext(ctx,
			accountID, f.Name, f.Path, delim, string(attrs),
			f.UIDValidity, f.UIDNext, f.HighestModSeq,
			f.TotalCount, f.UnreadCount, f.LastSyncedAt.Unix()); err != nil {
			return fmt.Errorf("store: upsert folder %q: %w", f.Path, err)
		}
	}
	return tx.Commit()
}

func (s *Store) ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, account_id, name, path, delimiter, attributes, uid_validity,
		        uid_next, highest_modseq, total_count, unread_count, last_synced_at
		 FROM folders WHERE account_id = ? ORDER BY path`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list folders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.Folder
	for rows.Next() {
		var (
			f      model.Folder
			attrs  string
			synced int64
		)
		if err := rows.Scan(&f.ID, &f.AccountID, &f.Name, &f.Path, &f.Delimiter,
			&attrs, &f.UIDValidity, &f.UIDNext, &f.HighestModSeq,
			&f.TotalCount, &f.UnreadCount, &synced); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(attrs, &f.Attributes); err != nil {
			return nil, fmt.Errorf("store: decode attributes for %q: %w", f.Path, err)
		}
		f.LastSyncedAt = time.Unix(synced, 0)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ResetFolder discards every locally cached message for one folder and records
// the server's new UIDVALIDITY.
//
// Called when the server changes UIDVALIDITY, which means every UID we hold
// for that mailbox now refers to a different message. Scoped to a single
// folder on purpose: its siblings' caches are still valid, and a test asserts
// the reset does not reach them.
//
// The FTS index needs no attention here — deleting the rows fires the delete
// trigger for each one.
func (s *Store) ResetFolder(ctx context.Context, folderID int64, newUIDValidity uint32) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin folder reset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM messages WHERE folder_id = ?`, folderID); err != nil {
		return fmt.Errorf("store: clear folder %d: %w", folderID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE folders
		 SET uid_validity = ?, uid_next = 0, highest_modseq = 0, last_synced_at = 0
		 WHERE id = ?`, newUIDValidity, folderID); err != nil {
		return fmt.Errorf("store: reset folder %d state: %w", folderID, err)
	}
	return tx.Commit()
}
