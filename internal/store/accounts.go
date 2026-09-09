package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

// ErrNotFound reports that no row matched.
var ErrNotFound = errors.New("store: not found")

const accountColumns = `id, email, display_name, provider, auth_kind,
	imap_host, imap_port, smtp_host, smtp_port, secret_ref, created_at`

// InsertAccount records a new account. The secret is not part of Account's
// stored form: only SecretRef, which names an entry in the SecretStore.
func (s *Store) InsertAccount(ctx context.Context, a model.Account) (int64, error) {
	res, err := s.write.ExecContext(ctx,
		`INSERT INTO accounts
		   (email, display_name, provider, auth_kind, imap_host, imap_port,
		    smtp_host, smtp_port, secret_ref, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Email, a.DisplayName, string(a.Provider), string(a.AuthKind),
		a.IMAPHost, a.IMAPPort, a.SMTPHost, a.SMTPPort,
		a.SecretRef, a.CreatedAt.Unix())
	if err != nil {
		return 0, fmt.Errorf("store: insert account %q: %w", a.Email, err)
	}
	return res.LastInsertId()
}

func (s *Store) ListAccounts(ctx context.Context) ([]model.Account, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAccount resolves one account by id, returning ErrNotFound when there is
// no such row. Callers resolve accounts by id constantly; a zero-valued
// account that silently does nothing would be far harder to debug.
func (s *Store) GetAccount(ctx context.Context, id int64) (model.Account, error) {
	row := s.read.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id)

	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Account{}, fmt.Errorf("store: account %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return model.Account{}, err
	}
	return a, nil
}

// scanner covers both *sql.Row and *sql.Rows so one scan function serves both.
type scanner interface {
	Scan(dest ...any) error
}

func scanAccount(sc scanner) (model.Account, error) {
	var (
		a                  model.Account
		provider, authKind string
		createdAt          int64
	)
	if err := sc.Scan(&a.ID, &a.Email, &a.DisplayName, &provider, &authKind,
		&a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort,
		&a.SecretRef, &createdAt); err != nil {
		return model.Account{}, err
	}
	a.Provider = model.Provider(provider)
	a.AuthKind = model.AuthKind(authKind)
	a.CreatedAt = time.Unix(createdAt, 0)
	return a, nil
}
