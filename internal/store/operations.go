package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

// opPayload is the shape of the operations.payload column.
//
// The varying parts of an operation live in JSON rather than in columns
// because they differ per kind: a move needs a destination, a flag change
// needs flags, a delete needs neither. Columns for all of them would be mostly
// NULL and would invite writing a move with no destination.
type opPayload struct {
	UIDs           []uint32 `json:"uids"`
	Flags          []string `json:"flags,omitempty"`
	TargetFolderID int64    `json:"targetFolderId,omitempty"`

	// The send fields. A name rather than a path, because where the outbox
	// directory lives is the host's business and a stored absolute path would
	// break the first time the profile moved to another machine.
	Outbox       string   `json:"outbox,omitempty"`
	EnvelopeFrom string   `json:"envelopeFrom,omitempty"`
	EnvelopeTo   []string `json:"envelopeTo,omitempty"`
}

// EnqueueOperation records a change to be sent to the server.
//
// The caller stamps the operation with the folder's current UIDValidity. That
// stamp is what the worker checks before sending: a mailbox the server has
// recreated has a new UID space, and the same number then names a different
// message. Queueing by Message-ID instead would avoid the problem and create a
// worse one — IMAP has no "act on this Message-ID" command, so every operation
// would need a SEARCH first, on a header not every server indexes.
func (s *Store) EnqueueOperation(ctx context.Context, op model.Operation) (int64, error) {
	if op.Kind.ActsOnUIDs() && len(op.UIDs) == 0 {
		return 0, errors.New("store: an operation must name at least one UID")
	}
	if op.Kind == "" {
		return 0, errors.New("store: an operation must have a kind")
	}
	// A UID only means something inside one folder. Caught here rather than as
	// a foreign key violation three frames down, where the message says
	// nothing about what was wrong.
	//
	// A send names no folder, because the message has not reached one: where
	// the copy is filed is decided after the server accepts it.
	if op.Kind.NeedsFolder() && op.FolderID == 0 {
		return 0, errors.New("store: an operation must name the folder its UIDs belong to")
	}
	if op.Kind == model.OpSend {
		if op.Outbox == "" {
			return 0, errors.New("store: a send must name the file holding the message")
		}
		if op.EnvelopeFrom == "" || len(op.EnvelopeTo) == 0 {
			return 0, errors.New("store: a send needs a return path and at least one recipient")
		}
	}

	payload, err := json.Marshal(opPayload{
		UIDs:           op.UIDs,
		Flags:          op.Flags,
		TargetFolderID: op.TargetFolderID,
		Outbox:         op.Outbox,
		EnvelopeFrom:   op.EnvelopeFrom,
		EnvelopeTo:     op.EnvelopeTo,
	})
	if err != nil {
		return 0, fmt.Errorf("store: encoding operation payload: %w", err)
	}

	created := op.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}

	res, err := s.write.ExecContext(ctx,
		`INSERT INTO operations
		   (account_id, folder_id, uid_validity, kind, payload, state,
		    attempts, last_error, created_at, next_attempt_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, '', ?, 0)`,
		op.AccountID, nullableFolder(op), op.UIDValidity, string(op.Kind),
		string(payload), string(model.OpPending), created.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("store: queueing %s operation: %w", op.Kind, err)
	}
	return res.LastInsertId()
}

// ClaimOperations returns the pending work for one account, oldest first.
//
// "Oldest first" is not cosmetic. Two changes to the same message applied out
// of order leave it in the state the user asked for second-to-last.
//
// Operations whose retry time has not arrived are skipped rather than
// returned. That is what keeps one broken change from freezing an account: the
// queue steps over it and the rest keeps flowing.
func (s *Store) ClaimOperations(ctx context.Context, accountID int64, limit int) ([]model.Operation, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, account_id, folder_id, uid_validity, kind, payload,
		        state, attempts, last_error, created_at, next_attempt_at
		   FROM operations
		  WHERE account_id = ? AND state = ? AND next_attempt_at <= ?
		  ORDER BY created_at, id
		  LIMIT ?`,
		accountID, string(model.OpPending), time.Now().UnixNano(), limit)
	if err != nil {
		return nil, fmt.Errorf("store: claiming operations for account %d: %w", accountID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.Operation
	for rows.Next() {
		var (
			op                model.Operation
			kind, state       string
			payload           string
			createdAt, nextAt int64
			folderID          *int64
		)
		if err := rows.Scan(&op.ID, &op.AccountID, &folderID, &op.UIDValidity,
			&kind, &payload, &state, &op.Attempts, &op.LastError,
			&createdAt, &nextAt); err != nil {
			return nil, err
		}

		var p opPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return nil, fmt.Errorf("store: operation %d has an unreadable payload: %w", op.ID, err)
		}

		if folderID != nil {
			op.FolderID = *folderID
		}
		op.Kind = model.OperationKind(kind)
		op.State = model.OperationState(state)
		op.UIDs = p.UIDs
		op.Flags = p.Flags
		op.TargetFolderID = p.TargetFolderID
		op.Outbox = p.Outbox
		op.EnvelopeFrom = p.EnvelopeFrom
		op.EnvelopeTo = p.EnvelopeTo
		op.CreatedAt = time.Unix(0, createdAt)
		op.NextAttemptAt = time.Unix(0, nextAt)

		out = append(out, op)
	}
	return out, rows.Err()
}

// MarkOperationDone records that the server accepted the change.
func (s *Store) MarkOperationDone(ctx context.Context, id int64) error {
	return s.setOperationState(ctx, id, model.OpDone, "")
}

// MarkOperationDropped records that the change could not be applied because
// the mailbox it was queued against no longer exists in that form.
//
// Kept rather than deleted so the user can be told. Silently discarding it
// would turn a visible loss of information into an invisible loss of their
// intent, which is the worse of the two.
func (s *Store) MarkOperationDropped(ctx context.Context, id int64) error {
	return s.setOperationState(ctx, id, model.OpDropped, "")
}

// MarkOperationPermanentlyFailed stops retrying and keeps the reason.
func (s *Store) MarkOperationPermanentlyFailed(ctx context.Context, id int64, reason string) error {
	return s.setOperationState(ctx, id, model.OpFailed, reason)
}

func (s *Store) setOperationState(ctx context.Context, id int64, state model.OperationState, reason string) error {
	_, err := s.write.ExecContext(ctx,
		`UPDATE operations SET state = ?, last_error = ? WHERE id = ?`,
		string(state), reason, id)
	if err != nil {
		return fmt.Errorf("store: marking operation %d as %s: %w", id, state, err)
	}
	return nil
}

// MarkOperationFailed counts the attempt and pushes the retry into the future.
//
// The operation stays pending: this is for the failures worth trying again —
// a dropped connection, a server that is briefly unhappy. Deciding when to
// stop belongs to the worker, which is where the error classes live.
func (s *Store) MarkOperationFailed(ctx context.Context, id int64, reason string, retryAt time.Time) error {
	_, err := s.write.ExecContext(ctx,
		`UPDATE operations
		    SET attempts = attempts + 1, last_error = ?, next_attempt_at = ?
		  WHERE id = ?`,
		reason, retryAt.UnixNano(), id)
	if err != nil {
		return fmt.Errorf("store: recording a failed attempt for operation %d: %w", id, err)
	}
	return nil
}

// CountOperations reports how many of an account's operations are in a state.
// Used to tell the user how many changes never made it.
func (s *Store) CountOperations(ctx context.Context, accountID int64, state model.OperationState) (int, error) {
	var n int
	err := s.read.QueryRowContext(ctx,
		`SELECT count(*) FROM operations WHERE account_id = ? AND state = ?`,
		accountID, string(state)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: counting %s operations: %w", state, err)
	}
	return n, nil
}

// ForgetFinishedOperations removes an account's dropped, failed and completed
// operations.
//
// This is what makes the "N changes could not be applied" notice dismissible.
// Hiding it in the interface while the rows stayed would mean the window and
// the database disagreed about whether the user had been told; deleting the
// rows makes the count itself the answer.
//
// Pending work is untouched. Acknowledging a failure is not the same as
// cancelling everything else the user asked for.
func (s *Store) ForgetFinishedOperations(ctx context.Context, accountID int64) error {
	_, err := s.write.ExecContext(ctx,
		`DELETE FROM operations WHERE account_id = ? AND state IN (?, ?, ?)`,
		accountID, string(model.OpDone), string(model.OpDropped), string(model.OpFailed))
	if err != nil {
		return fmt.Errorf("store: clearing finished operations: %w", err)
	}
	return nil
}

// nullableFolder writes NULL for an operation that belongs to no mailbox.
//
// Zero would be a foreign key to a folder that does not exist, and the column
// is nullable precisely so that a send can say "none" rather than "folder 0".
func nullableFolder(op model.Operation) any {
	if !op.Kind.NeedsFolder() || op.FolderID == 0 {
		return nil
	}
	return op.FolderID
}
