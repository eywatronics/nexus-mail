package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

const draftColumns = `id, account_id, identity_id, to_line, cc_line, bcc_line,
	subject, body, in_reply_to, refs, attachment_paths, updated_at`

// SaveDraft writes a draft, creating it the first time and replacing it after.
//
// The caller passes back the id it was given, which is what makes autosave one
// row rather than one row per pause. A zero id is a new draft.
func (s *Store) SaveDraft(ctx context.Context, d model.Draft) (int64, error) {
	refs, err := json.Marshal(d.References)
	if err != nil {
		return 0, fmt.Errorf("store: encode draft references: %w", err)
	}
	paths, err := json.Marshal(d.AttachmentPaths)
	if err != nil {
		return 0, fmt.Errorf("store: encode draft attachments: %w", err)
	}

	now := time.Now().Unix()
	if d.ID == 0 {
		res, err := s.write.ExecContext(ctx,
			`INSERT INTO drafts
			   (account_id, identity_id, to_line, cc_line, bcc_line, subject, body,
			    in_reply_to, refs, attachment_paths, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.AccountID, d.IdentityID, d.To, d.Cc, d.Bcc, d.Subject, d.Body,
			d.InReplyTo, string(refs), string(paths), now)
		if err != nil {
			return 0, fmt.Errorf("store: insert draft: %w", err)
		}
		return res.LastInsertId()
	}

	res, err := s.write.ExecContext(ctx,
		`UPDATE drafts SET identity_id = ?, to_line = ?, cc_line = ?, bcc_line = ?,
		   subject = ?, body = ?, in_reply_to = ?, refs = ?, attachment_paths = ?,
		   updated_at = ?
		 WHERE id = ? AND account_id = ?`,
		d.IdentityID, d.To, d.Cc, d.Bcc, d.Subject, d.Body, d.InReplyTo,
		string(refs), string(paths), now, d.ID, d.AccountID)
	if err != nil {
		return 0, fmt.Errorf("store: update draft %d: %w", d.ID, err)
	}

	// A draft the user deleted in another window, or one belonging to an
	// account that has since gone, must not silently turn into a no-op that
	// the composer reads as a successful save.
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: update draft %d: %w", d.ID, err)
	}
	if affected == 0 {
		return 0, fmt.Errorf("store: draft %d is gone: %w", d.ID, ErrNotFound)
	}
	return d.ID, nil
}

// ListDrafts returns an account's drafts, most recently touched first.
func (s *Store) ListDrafts(ctx context.Context, accountID int64) ([]model.Draft, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT `+draftColumns+` FROM drafts
		  WHERE account_id = ? ORDER BY updated_at DESC, id DESC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.Draft
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDraft reads one draft back, for reopening it in the composer.
func (s *Store) GetDraft(ctx context.Context, id int64) (model.Draft, error) {
	row := s.read.QueryRowContext(ctx,
		`SELECT `+draftColumns+` FROM drafts WHERE id = ?`, id)

	d, err := scanDraft(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Draft{}, fmt.Errorf("store: no draft %d: %w", id, ErrNotFound)
	}
	return d, err
}

// DeleteDraft removes one. Deleting a draft that is already gone is not an
// error: sending the same message twice from two windows would otherwise fail
// the second time, after the message had already gone out.
func (s *Store) DeleteDraft(ctx context.Context, id int64) error {
	if _, err := s.write.ExecContext(ctx, `DELETE FROM drafts WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete draft %d: %w", id, err)
	}
	return nil
}

func scanDraft(row scanner) (model.Draft, error) {
	var (
		d           model.Draft
		refs, paths string
		updated     int64
	)
	if err := row.Scan(&d.ID, &d.AccountID, &d.IdentityID, &d.To, &d.Cc, &d.Bcc,
		&d.Subject, &d.Body, &d.InReplyTo, &refs, &paths, &updated); err != nil {
		return model.Draft{}, err
	}
	if err := unmarshalIfSet(refs, &d.References); err != nil {
		return model.Draft{}, err
	}
	if err := unmarshalIfSet(paths, &d.AttachmentPaths); err != nil {
		return model.Draft{}, err
	}
	d.UpdatedAt = time.Unix(updated, 0)
	return d, nil
}
