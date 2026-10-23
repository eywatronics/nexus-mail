package store

import (
	"context"
	"fmt"
	"strings"

	"nexusmail/internal/model"
)

// SuggestRecipients returns people already in this account's local mail, for
// the composer's address completion.
//
// The senders of mail already synced are the only address book this build has:
// contacts arrive in M7. That is a real limit and the right one for now — an
// address the account has never corresponded with would have to come from
// somewhere this client does not have.
//
// One row per address, carrying the name from the most recent message. People
// change how they sign, and the newest spelling is the one the reader saw last.
func (s *Store) SuggestRecipients(ctx context.Context, accountID int64, query string, limit int) ([]model.Address, error) {
	query = strings.TrimSpace(query)
	if query == "" || limit <= 0 {
		return nil, nil
	}

	pattern := "%" + escapeLike(query) + "%"
	rows, err := s.read.QueryContext(ctx, `
		SELECT from_name, from_addr
		FROM (
			SELECT from_name, from_addr, internal_date,
			       ROW_NUMBER() OVER (
			           PARTITION BY lower(from_addr)
			           ORDER BY internal_date DESC, id DESC
			       ) AS rn
			FROM messages
			WHERE account_id = ?
			  AND from_addr <> ''
			  AND (from_addr LIKE ? ESCAPE '\' OR from_name LIKE ? ESCAPE '\')
		)
		WHERE rn = 1
		ORDER BY internal_date DESC
		LIMIT ?`, accountID, pattern, pattern, limit)
	if err != nil {
		// The query text is not in this error: it is what the person typed.
		return nil, fmt.Errorf("store: suggest recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Address, 0, limit)
	for rows.Next() {
		var a model.Address
		if err := rows.Scan(&a.Name, &a.Addr); err != nil {
			return nil, fmt.Errorf("store: scan suggested recipient: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read suggested recipients: %w", err)
	}
	return out, nil
}

// escapeLike makes user input literal inside a LIKE pattern.
//
// Without it a percent sign in what somebody typed would be a wildcard, and a
// search for "100%" would return every address in the mailbox.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
