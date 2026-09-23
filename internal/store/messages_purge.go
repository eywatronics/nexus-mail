package store

import (
	"context"
	"fmt"
	"time"

	"nexusmail/internal/model"
)

// PurgeFolder removes messages that fall outside the retention window and
// reports how many went.
//
// Three properties this deliberately has:
//
// It is local only. Nothing here writes to the operations queue and nothing
// reaches the server. Confusing housekeeping on our own disk with an IMAP
// delete would mean a client destroying years of someone's mail because their
// laptop was short of space.
//
// Flagged messages are exempt, whatever their age or position. A message the
// user deliberately marked is not something to quietly delete for disk space,
// and a client that did would be one nobody could trust with an archive.
//
// A policy with no limits does nothing. On a local-first client the disk is
// the user's to spend, and "keep everything" has to be a real choice.
//
// The candidate rows are read before deleting rather than expressed as one
// DELETE. Flags live in a JSON column, and deciding what "flagged" means in
// SQL would put a second, subtly different answer next to model.HasFlag —
// which already handles the capitalisation servers disagree about.
func (s *Store) PurgeFolder(ctx context.Context, folderID int64, policy model.RetentionPolicy) (int, error) {
	if !policy.Enabled() {
		return 0, nil
	}

	doomed, err := s.expiredUIDs(ctx, folderID, policy)
	if err != nil {
		return 0, err
	}
	if len(doomed) == 0 {
		return 0, nil
	}
	if err := s.DeleteMessagesByUID(ctx, folderID, doomed); err != nil {
		return 0, err
	}
	return len(doomed), nil
}

// expiredUIDs lists the UIDs a policy would remove.
//
// Rows come back newest first, which is what makes the count limit a matter of
// position: everything past the cap is out. The age test is applied to the
// same pass because a message can fail either limit independently — the window
// is the intersection, not a filter chain.
func (s *Store) expiredUIDs(ctx context.Context, folderID int64, policy model.RetentionPolicy) ([]uint32, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT uid, internal_date, flags
		   FROM messages
		  WHERE folder_id = ?
		  ORDER BY internal_date DESC, uid DESC`, folderID)
	if err != nil {
		return nil, fmt.Errorf("store: reading folder %d for purge: %w", folderID, err)
	}
	defer func() { _ = rows.Close() }()

	var (
		cutoff  = time.Now().Add(-policy.MaxAge)
		kept    int
		expired []uint32
	)

	for rows.Next() {
		var (
			uid      uint32
			internal int64
			flags    string
		)
		if err := rows.Scan(&uid, &internal, &flags); err != nil {
			return nil, err
		}

		var msg model.Message
		if err := unmarshalIfSet(flags, &msg.Flags); err != nil {
			return nil, err
		}
		// Exempt before anything else: a flagged message never counts against
		// the cap either, or a folder full of them would push out the mail the
		// user actually wants kept.
		if msg.HasFlag(model.FlagFlagged) {
			continue
		}

		tooOld := policy.MaxAge > 0 && time.Unix(internal, 0).Before(cutoff)
		beyondCap := policy.MaxMessages > 0 && kept >= policy.MaxMessages

		if tooOld || beyondCap {
			expired = append(expired, uid)
			continue
		}
		kept++
	}
	return expired, rows.Err()
}
