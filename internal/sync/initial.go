package sync

import (
	"context"
	"fmt"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// eagerAttributes marks the folders worth syncing headers for immediately.
// Everything else waits until the user opens it.
var eagerAttributes = []string{"\\Sent", "\\Drafts", "\\Trash", "\\Junk", "\\Archive"}

// InitialSync discovers folders, then fetches headers for the inbox and the
// special folders. Ordinary folders are recorded but left empty; SyncFolder
// fills them in on first open.
func (e *Engine) InitialSync(ctx context.Context, acct model.Account) error {
	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}

	remote, err := be.ListFolders(ctx)
	if err != nil {
		return fmt.Errorf("sync: listing folders for account %d: %w", acct.ID, err)
	}
	if err := e.store.UpsertFolders(ctx, acct.ID, remote); err != nil {
		return fmt.Errorf("sync: storing folders: %w", err)
	}

	// Re-read from the store: the remote list has no local ids, and every
	// subsequent write is keyed on them.
	local, err := e.store.ListFolders(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: reading stored folders: %w", err)
	}

	for _, folder := range local {
		if !isEager(folder) {
			continue
		}
		if err := e.syncFolderHeaders(ctx, be, acct.ID, folder); err != nil {
			return err
		}
	}
	return nil
}

// SyncFolder fetches headers for one folder on demand, used when the user
// opens a folder that the initial sync deliberately left empty.
func (e *Engine) SyncFolder(ctx context.Context, acct model.Account, folder model.Folder) error {
	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}
	return e.syncFolderHeaders(ctx, be, acct.ID, folder)
}

// syncFolderHeaders selects a folder, reconciles UIDVALIDITY and writes
// headers.
//
// The UIDVALIDITY check must happen before any message is written. If the
// server has recreated the mailbox, every UID we hold refers to a different
// message than it used to, and writing against the stale rows would attach new
// headers to old ids — or, later, have an operation act on the wrong mail.
func (e *Engine) syncFolderHeaders(ctx context.Context, be imapx.MailBackend, accountID int64, folder model.Folder) error {
	sel, err := be.Select(ctx, folder.Path)
	if err != nil {
		return fmt.Errorf("sync: selecting %q: %w", folder.Path, err)
	}

	// A zero stored value means this folder has never been synced, so there is
	// nothing to invalidate.
	if folder.UIDValidity != 0 && sel.UIDValidity != folder.UIDValidity {
		if err := e.store.ResetFolder(ctx, folder.ID, sel.UIDValidity); err != nil {
			return fmt.Errorf("sync: resetting %q after a UIDVALIDITY change: %w", folder.Path, err)
		}
		folder.UIDValidity = sel.UIDValidity
	}

	msgs, err := be.FetchHeaders(ctx, initialRange(sel.UIDNext))
	if err != nil {
		return fmt.Errorf("sync: fetching headers for %q: %w", folder.Path, err)
	}

	for i := range msgs {
		msgs[i].AccountID = accountID
		msgs[i].FolderID = folder.ID
	}
	if err := e.store.UpsertMessages(ctx, folder.ID, msgs); err != nil {
		return fmt.Errorf("sync: storing headers for %q: %w", folder.Path, err)
	}

	updated := folder
	updated.UIDNext = sel.UIDNext
	updated.HighestModSeq = sel.HighestModSeq
	updated.LastSyncedAt = time.Now()
	if err := e.store.UpsertFolders(ctx, accountID, []model.Folder{updated}); err != nil {
		return fmt.Errorf("sync: recording sync state for %q: %w", folder.Path, err)
	}
	return nil
}

// initialRange asks for the most recent InitialHeaderCount UIDs rather than
// the whole mailbox. A ten-year archive is not worth downloading before the
// user has seen their inbox.
func initialRange(uidNext uint32) imapx.UIDRange {
	start := uint32(1)
	if uidNext > InitialHeaderCount {
		start = uidNext - InitialHeaderCount
	}
	return imapx.UIDRange{Start: start}
}

func isEager(f model.Folder) bool {
	if f.IsInbox() {
		return true
	}
	for _, eager := range eagerAttributes {
		if f.HasAttribute(eager) {
			return true
		}
	}
	return false
}
