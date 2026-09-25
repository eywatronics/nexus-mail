package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

const identityColumns = `id, account_id, email, display_name, reply_to,
	signature_text, signature_html, is_default, sort_order, created_at`

// InsertIdentity adds an address an account can send as.
//
// The first identity on an account becomes its default whatever the caller
// asked for. An account with identities but no default is one the composer
// cannot open, and "the caller should have set it" is not a guarantee — it is
// a hope.
func (s *Store) InsertIdentity(ctx context.Context, i model.Identity) (int64, error) {
	if i.Email == "" {
		return 0, errors.New("store: an identity needs an address")
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now()
	}

	var id int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var existing int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM identities WHERE account_id = ?`,
			i.AccountID).Scan(&existing); err != nil {
			return err
		}
		if existing == 0 {
			i.IsDefault = true
		}
		if i.IsDefault {
			// Cleared first, because the schema refuses a second default and
			// would otherwise turn "make this the default" into an error.
			if _, err := tx.ExecContext(ctx,
				`UPDATE identities SET is_default = 0 WHERE account_id = ?`,
				i.AccountID); err != nil {
				return err
			}
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO identities
			   (account_id, email, display_name, reply_to,
			    signature_text, signature_html, is_default, sort_order, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			i.AccountID, i.Email, i.DisplayName, i.ReplyTo,
			i.SignatureText, i.SignatureHTML, boolToInt(i.IsDefault),
			i.SortOrder, i.CreatedAt.Unix())
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("store: insert identity %q: %w", i.Email, err)
	}
	return id, nil
}

// ListIdentities returns an account's identities in the order it wants them
// shown, default first within that order.
func (s *Store) ListIdentities(ctx context.Context, accountID int64) ([]model.Identity, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT `+identityColumns+` FROM identities
		  WHERE account_id = ?
		  ORDER BY is_default DESC, sort_order, id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list identities: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.Identity
	for rows.Next() {
		i, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DefaultIdentity is the one a new message is from.
func (s *Store) DefaultIdentity(ctx context.Context, accountID int64) (model.Identity, error) {
	row := s.read.QueryRowContext(ctx,
		`SELECT `+identityColumns+` FROM identities
		  WHERE account_id = ? AND is_default = 1`, accountID)

	i, err := scanIdentity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Identity{}, fmt.Errorf("store: account %d has no default identity: %w",
			accountID, ErrNotFound)
	}
	if err != nil {
		return model.Identity{}, fmt.Errorf("store: default identity: %w", err)
	}
	return i, nil
}

// SetDefaultIdentity moves the default to one identity of the same account.
//
// The move is one transaction because the schema allows only one default at a
// time: clearing and setting in two statements would leave a moment with none,
// and a composer opened in that moment has nothing to send as.
func (s *Store) SetDefaultIdentity(ctx context.Context, identityID int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var accountID int64
		if err := tx.QueryRowContext(ctx,
			`SELECT account_id FROM identities WHERE id = ?`, identityID).Scan(&accountID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("store: no identity %d: %w", identityID, ErrNotFound)
			}
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE identities SET is_default = 0 WHERE account_id = ?`, accountID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE identities SET is_default = 1 WHERE id = ?`, identityID)
		return err
	})
}

// UpdateIdentity changes everything about an identity except which account it
// belongs to and whether it is the default. Moving an identity between
// accounts is not an edit, and the default is moved by SetDefaultIdentity so
// that clearing and setting stay one transaction.
func (s *Store) UpdateIdentity(ctx context.Context, i model.Identity) error {
	if i.Email == "" {
		return errors.New("store: an identity needs an address")
	}

	res, err := s.write.ExecContext(ctx,
		`UPDATE identities
		    SET email = ?, display_name = ?, reply_to = ?,
		        signature_text = ?, signature_html = ?, sort_order = ?
		  WHERE id = ?`,
		i.Email, i.DisplayName, i.ReplyTo,
		i.SignatureText, i.SignatureHTML, i.SortOrder, i.ID)
	if err != nil {
		return fmt.Errorf("store: update identity %d: %w", i.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("store: no identity %d: %w", i.ID, ErrNotFound)
	}
	return nil
}

// DeleteIdentity removes one, and refuses to remove the last.
//
// An account with no identity cannot open a composer, and the error says so
// rather than leaving the caller to discover it later. Deleting the default
// hands the role to the next in order, because leaving an account with
// identities and no default is the same broken state by another route.
func (s *Store) DeleteIdentity(ctx context.Context, identityID int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var accountID int64
		var wasDefault int
		err := tx.QueryRowContext(ctx,
			`SELECT account_id, is_default FROM identities WHERE id = ?`,
			identityID).Scan(&accountID, &wasDefault)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: no identity %d: %w", identityID, ErrNotFound)
		}
		if err != nil {
			return err
		}

		var remaining int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM identities WHERE account_id = ?`,
			accountID).Scan(&remaining); err != nil {
			return err
		}
		if remaining <= 1 {
			return fmt.Errorf(
				"store: identity %d is the only one on its account; an account with none "+
					"cannot send", identityID)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM identities WHERE id = ?`, identityID); err != nil {
			return err
		}
		if wasDefault == 0 {
			return nil
		}
		// The role goes to the next in the order the user chose, not to the
		// lowest id: they arranged the list, and the arrangement is the answer.
		_, err = tx.ExecContext(ctx,
			`UPDATE identities SET is_default = 1
			  WHERE id = (SELECT id FROM identities WHERE account_id = ?
			               ORDER BY sort_order, id LIMIT 1)`, accountID)
		return err
	})
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanIdentity(row rowScanner) (model.Identity, error) {
	var i model.Identity
	var isDefault int
	var created int64

	if err := row.Scan(&i.ID, &i.AccountID, &i.Email, &i.DisplayName, &i.ReplyTo,
		&i.SignatureText, &i.SignatureHTML, &isDefault, &i.SortOrder, &created); err != nil {
		return model.Identity{}, err
	}
	i.IsDefault = isDefault == 1
	i.CreatedAt = time.Unix(created, 0)
	return i, nil
}
