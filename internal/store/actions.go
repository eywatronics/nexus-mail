package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nexusmail/internal/model"
)

// targetMessage is one message an action applies to, with everything the
// action needs about it.
type targetMessage struct {
	id          int64
	accountID   int64
	folderID    int64
	uid         uint32
	uidValidity uint32
	flags       []string
}

// ApplyFlagChange sets or clears flags locally and queues the same change for
// the server, in one transaction.
//
// Both halves or neither. Writing the local change alone leaves the window
// showing a message as read that the server will never hear about, and the
// user finds out a week later on their phone. Queueing alone leaves the window
// showing the opposite of what is about to happen.
//
// Messages already in the requested state are skipped entirely. The reading
// pane marks messages read constantly; a queue that grew on every redundant
// call would send thousands of commands that change nothing.
func (s *Store) ApplyFlagChange(ctx context.Context, messageIDs []int64, flags []string, add bool) error {
	if len(messageIDs) == 0 || len(flags) == 0 {
		return nil
	}

	kind := model.OpRemoveFlags
	if add {
		kind = model.OpAddFlags
	}

	return s.inTx(ctx, func(tx *sql.Tx) error {
		targets, err := loadTargets(ctx, tx, messageIDs)
		if err != nil {
			return err
		}

		byFolder := map[int64][]targetMessage{}
		for _, t := range targets {
			updated, changed := changeFlags(t.flags, flags, add)
			if !changed {
				continue
			}
			encoded, err := json.Marshal(updated)
			if err != nil {
				return fmt.Errorf("store: encoding flags for message %d: %w", t.id, err)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE messages SET flags = ? WHERE id = ?`, string(encoded), t.id); err != nil {
				return fmt.Errorf("store: updating flags for message %d: %w", t.id, err)
			}
			byFolder[t.folderID] = append(byFolder[t.folderID], t)
		}

		return queuePerFolder(ctx, tx, byFolder, kind, flags, 0)
	})
}

// ApplyMove removes messages from their folder locally and queues the move.
//
// The local row is deleted rather than repointed at the destination. The
// server assigns a new UID there, and a row carrying the old folder's UID
// would be wrong in a way the next sync could not fix — it would look like a
// message that exists, with a UID naming something else. The destination's
// next sync brings it back with the UID it really has.
func (s *Store) ApplyMove(ctx context.Context, messageIDs []int64, targetFolderID int64) error {
	if len(messageIDs) == 0 {
		return nil
	}

	return s.inTx(ctx, func(tx *sql.Tx) error {
		targets, err := loadTargets(ctx, tx, messageIDs)
		if err != nil {
			return err
		}

		byFolder := map[int64][]targetMessage{}
		for _, t := range targets {
			if t.folderID == targetFolderID {
				continue // already there
			}
			byFolder[t.folderID] = append(byFolder[t.folderID], t)
		}
		if err := deleteTargets(ctx, tx, byFolder); err != nil {
			return err
		}
		return queuePerFolder(ctx, tx, byFolder, model.OpMove, nil, targetFolderID)
	})
}

// ApplyDelete removes messages locally and queues the deletion.
func (s *Store) ApplyDelete(ctx context.Context, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}

	return s.inTx(ctx, func(tx *sql.Tx) error {
		targets, err := loadTargets(ctx, tx, messageIDs)
		if err != nil {
			return err
		}

		byFolder := map[int64][]targetMessage{}
		for _, t := range targets {
			byFolder[t.folderID] = append(byFolder[t.folderID], t)
		}
		if err := deleteTargets(ctx, tx, byFolder); err != nil {
			return err
		}
		return queuePerFolder(ctx, tx, byFolder, model.OpDelete, nil, 0)
	})
}

// inTx runs fn inside a write transaction, rolling back on any error.
func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// loadTargets reads what an action needs about each message, joined to its
// folder for the UIDVALIDITY stamp.
//
// Messages that are not there are simply absent from the result. A selection
// can name a message the retention window removed a moment ago, or one deleted
// on another device; neither is an error, the window just has a stale list.
func loadTargets(ctx context.Context, tx *sql.Tx, messageIDs []int64) ([]targetMessage, error) {
	args := make([]any, 0, len(messageIDs))
	for _, id := range messageIDs {
		args = append(args, id)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT m.id, m.account_id, m.folder_id, m.uid, f.uid_validity, m.flags
		   FROM messages m
		   JOIN folders f ON f.id = m.folder_id
		  WHERE m.id IN (`+placeholders(len(messageIDs))+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: reading the selected messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []targetMessage
	for rows.Next() {
		var (
			t     targetMessage
			flags string
		)
		if err := rows.Scan(&t.id, &t.accountID, &t.folderID, &t.uid, &t.uidValidity, &flags); err != nil {
			return nil, err
		}
		if err := unmarshalIfSet(flags, &t.flags); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// deleteTargets removes the named messages.
func deleteTargets(ctx context.Context, tx *sql.Tx, byFolder map[int64][]targetMessage) error {
	for _, targets := range byFolder {
		args := make([]any, 0, len(targets))
		for _, t := range targets {
			args = append(args, t.id)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM messages WHERE id IN (`+placeholders(len(targets))+`)`, args...); err != nil {
			return fmt.Errorf("store: removing messages locally: %w", err)
		}
	}
	return nil
}

// queuePerFolder writes one operation per folder.
//
// Per folder because a UID only means something inside one mailbox. A single
// operation covering a selection that spans folders would name UIDs from the
// wrong one, and the worker would act on whatever those numbers happen to be
// where it looked.
func queuePerFolder(ctx context.Context, tx *sql.Tx, byFolder map[int64][]targetMessage,
	kind model.OperationKind, flags []string, targetFolderID int64) error {

	for folderID, targets := range byFolder {
		if len(targets) == 0 {
			continue
		}

		uids := make([]uint32, 0, len(targets))
		for _, t := range targets {
			uids = append(uids, t.uid)
		}

		payload, err := json.Marshal(opPayload{
			UIDs: uids, Flags: flags, TargetFolderID: targetFolderID,
		})
		if err != nil {
			return fmt.Errorf("store: encoding operation payload: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO operations
			   (account_id, folder_id, uid_validity, kind, payload, state,
			    attempts, last_error, created_at, next_attempt_at)
			 VALUES (?, ?, ?, ?, ?, ?, 0, '', ?, 0)`,
			targets[0].accountID, folderID, targets[0].uidValidity, string(kind),
			string(payload), string(model.OpPending), nowNanos()); err != nil {
			return fmt.Errorf("store: queueing %s for folder %d: %w", kind, folderID, err)
		}
	}
	return nil
}

// changeFlags applies an add or remove and reports whether anything moved.
//
// Comparison is case-insensitive because servers disagree about how to
// capitalise system flags, the same reason model.HasFlag does.
func changeFlags(current, change []string, add bool) ([]string, bool) {
	has := func(list []string, f string) bool {
		for _, got := range list {
			if strings.EqualFold(got, f) {
				return true
			}
		}
		return false
	}

	if !add {
		kept := make([]string, 0, len(current))
		for _, f := range current {
			if !has(change, f) {
				kept = append(kept, f)
			}
		}
		return kept, len(kept) != len(current)
	}

	out := append([]string{}, current...)
	changed := false
	for _, f := range change {
		if !has(out, f) {
			out = append(out, f)
			changed = true
		}
	}
	return out, changed
}

// nowNanos is the clock the queue orders on, in one place so the two insert
// paths cannot drift.
func nowNanos() int64 { return time.Now().UnixNano() }
