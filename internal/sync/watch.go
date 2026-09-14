package sync

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// Reconnection schedule.
//
// The base is short because most drops are a laptop lid or a train tunnel and
// the connection comes straight back. The cap is what stops a client from
// giving up on a server that is down for an hour, and what stops it from
// hammering one that is down for a day.
const (
	backoffBase = 2 * time.Second
	backoffCap  = 5 * time.Minute

	// pollInterval is how often a server without IDLE is checked. Two minutes
	// is a compromise: often enough to feel live, rare enough that a hundred
	// clients do not look like traffic.
	pollInterval = 2 * time.Minute
)

// ErrNoInbox is returned when an account has no folder that looks like an
// inbox. There is nothing to IDLE on, so the loop would spin.
var ErrNoInbox = errors.New("sync: account has no inbox")

// Watch keeps one account in step with its server until ctx is cancelled.
//
// It returns nil on cancellation: shutting down is not a failure, and
// returning the context error would make every clean exit look like one in
// the logs.
//
// The loop is deliberately simple — connect, sync, wait for the server to
// speak, sync again — because the interesting part is what happens when it
// breaks. Every reconnect runs a full delta pass before waiting again, since
// nothing that changed while the connection was down was ever announced.
//
// onPass, if given, is called after each completed pass. It is how the window
// learns to redraw: the engine has no idea a user interface exists, and the
// caller has no idea when the server spoke.
func (e *Engine) Watch(ctx context.Context, acct model.Account, onPass func()) error {
	attempt := 0

	for {
		synced, err := e.watchOnce(ctx, acct, onPass)

		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, ErrNoInbox) {
			return err
		}

		// A connection that did useful work before dying starts the schedule
		// over. Without this, a server that drops the connection once an hour
		// would eventually be retried at the five-minute cap forever.
		if synced {
			attempt = 0
		} else {
			attempt++
		}

		select {
		case <-time.After(backoff(attempt)):
		case <-ctx.Done():
			return nil
		}
	}
}

// watchOnce holds one connection for as long as it lives, and reports whether
// it managed at least one sync before failing.
func (e *Engine) watchOnce(ctx context.Context, acct model.Account, onPass func()) (bool, error) {
	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return false, fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}
	defer func() { _ = be.Close() }()

	synced := false
	purged := false
	for {
		inbox, err := e.refreshFolders(ctx, be, acct)
		if err != nil {
			return synced, err
		}
		if inbox == nil {
			return synced, ErrNoInbox
		}
		synced = true

		// Outgoing changes go out before anything else is read. A queue
		// nothing drains is a queue that never reaches the server, and the
		// user's change would sit on disk looking applied.
		claimed, err := e.store.ClaimOperations(ctx, acct.ID, queueBatchSize)
		if err != nil {
			return synced, err
		}
		if len(claimed) > 0 {
			if err := e.drainOn(ctx, be, acct, claimed); err != nil {
				return synced, err
			}
		}

		// Once per connection, not once per wake. The purge walks every row in
		// a folder, and doing that each time a message arrives would turn a
		// twenty-five thousand row scan into a per-message cost. A connection
		// that lives a week overshoots the window by a few hundred rows, which
		// is noise against the cap — and the next reconnect settles it.
		if !purged {
			if err := e.purgeAccount(ctx, acct); err != nil {
				return synced, err
			}
			purged = true
		}

		if onPass != nil {
			onPass()
		}

		if err := e.waitForChange(ctx, be, *inbox); err != nil {
			return synced, err
		}
	}
}

// refreshFolders delta syncs everything already being tracked and returns the
// inbox, which is the mailbox the connection will then sit idle on.
//
// Folders the initial sync deliberately left empty are skipped. Syncing every
// folder on every wake would turn one new message into a scan of the whole
// account, and would fill in folders the user has never opened.
func (e *Engine) refreshFolders(ctx context.Context, be imapx.MailBackend, acct model.Account) (*model.Folder, error) {
	folders, err := e.store.ListFolders(ctx, acct.ID)
	if err != nil {
		return nil, err
	}

	var inbox *model.Folder
	for i := range folders {
		if folders[i].IsInbox() {
			inbox = &folders[i]
		}
		if folders[i].UIDNext == 0 {
			continue
		}
		if err := e.deltaSyncOn(ctx, be, acct, folders[i]); err != nil {
			return nil, err
		}
	}
	return inbox, nil
}

// purgeAccount applies the retention window to every folder that holds
// anything.
//
// Deliberately local: nothing here reaches the server. Housekeeping on our own
// disk and an IMAP delete are different operations, and confusing them would
// mean a client destroying years of someone's mail because their laptop was
// short of space.
func (e *Engine) purgeAccount(ctx context.Context, acct model.Account) error {
	policy := e.retentionPolicy()
	if !policy.Enabled() {
		return nil
	}

	folders, err := e.store.ListFolders(ctx, acct.ID)
	if err != nil {
		return err
	}
	for _, folder := range folders {
		if folder.UIDNext == 0 {
			continue
		}
		if _, err := e.store.PurgeFolder(ctx, folder.ID, policy); err != nil {
			return fmt.Errorf("sync: purging %q: %w", folder.Path, err)
		}
	}
	return nil
}

// waitForChange blocks until the server says something changed, or until the
// poll interval elapses on a server that cannot say so.
func (e *Engine) waitForChange(ctx context.Context, be imapx.MailBackend, inbox model.Folder) error {
	if !be.Capabilities().Idle {
		// Without IDLE the only option is to look again later. Returning an
		// error instead would send this through the reconnect path, which
		// would reopen the connection every couple of seconds.
		select {
		case <-time.After(pollInterval):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if _, err := be.Select(ctx, inbox.Path); err != nil {
		return fmt.Errorf("sync: selecting %q to idle on: %w", inbox.Path, err)
	}
	// The wake tells us something moved, not what. The next pass finds out.
	if _, err := be.Idle(ctx); err != nil {
		return fmt.Errorf("sync: idling on %q: %w", inbox.Path, err)
	}
	return nil
}

// backoff returns how long to wait before reconnect attempt n, counted from
// zero.
//
// Jitter is subtracted rather than added in either direction, so the result
// never climbs above the cap. Its purpose is the thundering herd: a thousand
// clients that lost the same server must not all come back on the same
// second, or the server's first act after recovering is to fall over again.
func backoff(attempt int) time.Duration {
	d := backoffBase
	for range attempt {
		d *= 2
		if d >= backoffCap {
			d = backoffCap
			break
		}
	}

	spread := int64(d) / 4
	if spread <= 0 {
		return d
	}
	return d - time.Duration(rand.Int64N(spread))
}
