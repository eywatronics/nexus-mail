package store

import (
	"context"
	"database/sql"
	"fmt"

	"nexusmail/internal/model"
)

// UpsertAttachments records what files a message carries.
//
// Called on every header sync, because the server describes the parts again
// each time. The conflict clause is what stops that from doubling the list:
// a part is identified by its number within its message, and the same number
// always means the same part for as long as the UID is valid.
//
// The downloaded file is deliberately not touched by the update. A resync that
// cleared it would make the reader download the same attachment again every
// time their mailbox was refreshed.
func (s *Store) UpsertAttachments(ctx context.Context, messageID int64, parts []model.AttachmentPart) error {
	if len(parts) == 0 {
		return nil
	}

	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin attachment upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO attachments (message_id, part_id, filename, mime_type, size, local_path, downloaded)
		 VALUES (?, ?, ?, ?, ?, '', 0)
		 ON CONFLICT(message_id, part_id) DO UPDATE SET
		   filename  = excluded.filename,
		   mime_type = excluded.mime_type,
		   size      = excluded.size`)
	if err != nil {
		return fmt.Errorf("store: prepare attachment upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, p := range parts {
		if _, err := stmt.ExecContext(ctx,
			messageID, p.PartID, p.Filename, p.MIMEType, p.Size); err != nil {
			return fmt.Errorf("store: storing attachment %s of message %d: %w",
				p.PartID, messageID, err)
		}
	}
	return tx.Commit()
}

// ListAttachments returns what a message carries, in the order the parts
// appear in it.
func (s *Store) ListAttachments(ctx context.Context, messageID int64) ([]model.AttachmentPart, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, part_id, filename, mime_type, size, local_path, downloaded
		   FROM attachments
		  WHERE message_id = ?
		  ORDER BY id`, messageID)
	if err != nil {
		return nil, fmt.Errorf("store: listing attachments of message %d: %w", messageID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.AttachmentPart
	for rows.Next() {
		var (
			p          model.AttachmentPart
			downloaded int
		)
		if err := rows.Scan(&p.ID, &p.PartID, &p.Filename, &p.MIMEType,
			&p.Size, &p.LocalPath, &downloaded); err != nil {
			return nil, err
		}
		p.Downloaded = downloaded == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetAttachment reads one attachment row, with the message and folder it needs
// to be fetched from.
func (s *Store) GetAttachment(ctx context.Context, id int64) (model.AttachmentPart, int64, error) {
	var (
		p          model.AttachmentPart
		messageID  int64
		downloaded int
	)
	err := s.read.QueryRowContext(ctx,
		`SELECT id, message_id, part_id, filename, mime_type, size, local_path, downloaded
		   FROM attachments WHERE id = ?`, id).
		Scan(&p.ID, &messageID, &p.PartID, &p.Filename, &p.MIMEType,
			&p.Size, &p.LocalPath, &downloaded)
	if err != nil {
		return model.AttachmentPart{}, 0, fmt.Errorf("store: no attachment %d: %w", id, err)
	}
	p.Downloaded = downloaded == 1
	return p, messageID, nil
}

// MarkAttachmentDownloaded records where the bytes were saved.
func (s *Store) MarkAttachmentDownloaded(ctx context.Context, id int64, localPath string) error {
	_, err := s.write.ExecContext(ctx,
		`UPDATE attachments SET local_path = ?, downloaded = 1 WHERE id = ?`, localPath, id)
	if err != nil {
		return fmt.Errorf("store: recording the download of attachment %d: %w", id, err)
	}
	return nil
}

// upsertAttachmentsTx writes the attachment lists carried by a batch of
// messages, inside the transaction that wrote the messages.
//
// The row ids are read back rather than taken from last_insert_rowid, because
// the upsert above takes the update path for a message we already had and
// there is no insert id to take.
func upsertAttachmentsTx(ctx context.Context, tx *sql.Tx, folderID int64, msgs []model.Message) error {
	wanted := map[uint32][]model.AttachmentPart{}
	for _, m := range msgs {
		if len(m.Attachments) > 0 {
			wanted[m.UID] = m.Attachments
		}
	}
	if len(wanted) == 0 {
		return nil
	}

	uids := make([]any, 0, len(wanted)+1)
	uids = append(uids, folderID)
	for uid := range wanted {
		uids = append(uids, uid)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, uid FROM messages WHERE folder_id = ? AND uid IN (`+
			placeholders(len(wanted))+`)`, uids...)
	if err != nil {
		return fmt.Errorf("store: resolving message ids for attachments: %w", err)
	}

	ids := map[uint32]int64{}
	for rows.Next() {
		var (
			id  int64
			uid uint32
		)
		if err := rows.Scan(&id, &uid); err != nil {
			_ = rows.Close()
			return err
		}
		ids[uid] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO attachments (message_id, part_id, filename, mime_type, size, local_path, downloaded)
		 VALUES (?, ?, ?, ?, ?, '', 0)
		 ON CONFLICT(message_id, part_id) DO UPDATE SET
		   filename  = excluded.filename,
		   mime_type = excluded.mime_type,
		   size      = excluded.size`)
	if err != nil {
		return fmt.Errorf("store: prepare attachment upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for uid, parts := range wanted {
		messageID, ok := ids[uid]
		if !ok {
			continue
		}
		for _, p := range parts {
			if _, err := stmt.ExecContext(ctx,
				messageID, p.PartID, p.Filename, p.MIMEType, p.Size); err != nil {
				return fmt.Errorf("store: storing attachment %s of UID %d: %w", p.PartID, uid, err)
			}
		}
	}
	return nil
}
