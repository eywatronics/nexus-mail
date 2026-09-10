package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

const messageColumns = `id, account_id, folder_id, uid, message_id, thread_id,
	in_reply_to, refs, subject, from_name, from_addr, to_addrs, cc_addrs,
	date, internal_date, size, snippet, flags, has_attachments, body_fetched`

// UpsertMessages writes a batch of message headers in one transaction.
//
// The UNIQUE(account_id, folder_id, uid) constraint plus ON CONFLICT is what
// makes reconnects safe: the same UID arriving twice updates the row instead of
// duplicating it.
//
// Nothing here touches fts_messages. The index is maintained by triggers on
// this table, and CI fails a build where application code writes to it —
// a write path that forgets the index is how search silently goes wrong.
func (s *Store) UpsertMessages(ctx context.Context, folderID int64, msgs []model.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin message upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO messages
		   (account_id, folder_id, uid, message_id, thread_id, in_reply_to, refs,
		    subject, from_name, from_addr, to_addrs, cc_addrs, date, internal_date,
		    size, snippet, flags, has_attachments, body_fetched)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, folder_id, uid) DO UPDATE SET
		   subject         = excluded.subject,
		   snippet         = excluded.snippet,
		   flags           = excluded.flags,
		   thread_id       = excluded.thread_id,
		   has_attachments = excluded.has_attachments`)
	if err != nil {
		return fmt.Errorf("store: prepare message upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, m := range msgs {
		refs, err := json.Marshal(m.References)
		if err != nil {
			return fmt.Errorf("store: encode references for UID %d: %w", m.UID, err)
		}
		to, err := json.Marshal(m.To)
		if err != nil {
			return fmt.Errorf("store: encode To for UID %d: %w", m.UID, err)
		}
		cc, err := json.Marshal(m.Cc)
		if err != nil {
			return fmt.Errorf("store: encode Cc for UID %d: %w", m.UID, err)
		}
		flags, err := json.Marshal(m.Flags)
		if err != nil {
			return fmt.Errorf("store: encode flags for UID %d: %w", m.UID, err)
		}

		if _, err := stmt.ExecContext(ctx,
			m.AccountID, folderID, m.UID, m.MessageID, m.ThreadID, m.InReplyTo,
			string(refs), m.Subject, m.From.Name, m.From.Addr, string(to), string(cc),
			m.Date.Unix(), m.InternalDate.Unix(), m.Size, m.Snippet, string(flags),
			boolToInt(m.HasAttachments), boolToInt(m.BodyFetched)); err != nil {
			return fmt.Errorf("store: upsert message UID %d: %w", m.UID, err)
		}
	}
	return tx.Commit()
}

// ListMessages returns one page of a folder, newest first.
//
// Ordering uses internal_date, the server's delivery time, rather than the
// Date: header — that header is attacker-controlled and frequently malformed,
// and sorting on it would let a spammer pin itself to the top of the inbox.
func (s *Store) ListMessages(ctx context.Context, folderID int64, limit, offset int) ([]model.Message, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT `+messageColumns+`
		 FROM messages
		 WHERE folder_id = ?
		 ORDER BY internal_date DESC, uid DESC
		 LIMIT ? OFFSET ?`, folderID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("store: list messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanMessageRows(rows)
}

// scanMessageRows decodes rows selected with messageColumns.
//
// Shared by every read path rather than copied into each one: the scan order
// has to match the column list exactly, and a second copy is a place for the
// two to drift the first time a field is added.
func scanMessageRows(rows *sql.Rows) ([]model.Message, error) {
	var out []model.Message
	for rows.Next() {
		var (
			m                   model.Message
			refs, to, cc, flags string
			date, internal      int64
			hasAtt, bodyFetched int
		)
		if err := rows.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.UID, &m.MessageID,
			&m.ThreadID, &m.InReplyTo, &refs, &m.Subject, &m.From.Name, &m.From.Addr,
			&to, &cc, &date, &internal, &m.Size, &m.Snippet, &flags,
			&hasAtt, &bodyFetched); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(refs, &m.References); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(to, &m.To); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(cc, &m.Cc); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(flags, &m.Flags); err != nil {
			return nil, err
		}
		m.Date = time.Unix(date, 0)
		m.InternalDate = time.Unix(internal, 0)
		m.HasAttachments = hasAtt == 1
		m.BodyFetched = bodyFetched == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

// unmarshalIfSet decodes JSON only when there is something to decode. Columns
// default to the empty string, and older rows may hold the literal "null".
func unmarshalIfSet(s string, dst any) error {
	if s == "" || s == "null" {
		return nil
	}
	return json.Unmarshal([]byte(s), dst)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
