package sync

import (
	"context"
	"fmt"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// DeltaSync brings one folder up to date without refetching what we already
// hold.
//
// Three things can have happened since the last pass: messages arrived, flags
// changed, messages were expunged. Each is found differently, and the third is
// the awkward one — an expunge does not advance HIGHESTMODSEQ, so a CONDSTORE
// query will never mention it.
func (e *Engine) DeltaSync(ctx context.Context, acct model.Account, folder model.Folder) error {
	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}
	return e.deltaSyncOn(ctx, be, acct, folder)
}

// deltaSyncOn is DeltaSync over a connection the caller already has.
//
// The watch loop holds one connection open for hours and syncs many folders
// through it; dialing again per folder would open a new connection for every
// message that arrives, and Gmail locks an account that does that.
func (e *Engine) deltaSyncOn(ctx context.Context, be imapx.MailBackend, acct model.Account, folder model.Folder) error {
	sel, err := be.Select(ctx, folder.Path)
	if err != nil {
		return fmt.Errorf("sync: selecting %q: %w", folder.Path, err)
	}

	folder, wasReset, err := e.reconcileUIDValidity(ctx, folder, sel)
	if err != nil {
		return err
	}

	// A reset mailbox has no UIDs left to delta against, and a folder the
	// initial sync deliberately left empty has none yet. Both need the full
	// window fetched before there is anything to compare on the next pass.
	if wasReset || folder.UIDNext == 0 {
		return e.fullHeaderPass(ctx, be, acct.ID, folder, sel)
	}

	arrived, err := e.fetchArrivals(ctx, be, acct.ID, folder, sel)
	if err != nil {
		return err
	}
	if err := e.reconcileWindow(ctx, be, folder, sel, arrived); err != nil {
		return err
	}

	return e.recordSyncState(ctx, acct.ID, folder, sel)
}

// fetchArrivals pulls headers for UIDs the server has grown past ours, and
// reports how many arrived.
//
// The count matters beyond the fetch itself: it is what lets the caller work
// out whether the mailbox's message count moved for a reason other than a
// deletion.
func (e *Engine) fetchArrivals(ctx context.Context, be imapx.MailBackend, accountID int64, folder model.Folder, sel imapx.SelectResult) (int, error) {
	if sel.UIDNext <= folder.UIDNext {
		return 0, nil
	}

	msgs, err := be.FetchHeaders(ctx, imapx.UIDRange{
		Start: folder.UIDNext,
		End:   sel.UIDNext - 1,
	})
	if err != nil {
		return 0, fmt.Errorf("sync: fetching new headers for %q: %w", folder.Path, err)
	}
	if err := e.storeHeaders(ctx, accountID, folder, msgs); err != nil {
		return 0, err
	}
	return len(msgs), nil
}

// reconcileWindow applies flag changes and deletions across the UIDs we hold.
//
// We keep a window of the mailbox, not all of it, so the range asked about is
// bounded by the oldest and newest UID actually stored. Asking about the whole
// mailbox would make the server describe years of mail we deliberately never
// downloaded.
func (e *Engine) reconcileWindow(ctx context.Context, be imapx.MailBackend, folder model.Folder, sel imapx.SelectResult, arrived int) error {
	local, err := e.store.ListMessageUIDs(ctx, folder.ID)
	if err != nil {
		return err
	}
	if len(local) == 0 {
		return nil
	}

	changedSince := condStoreSince(be.Capabilities(), folder, sel, arrived)

	updates, err := be.FetchFlags(ctx, imapx.UIDRange{
		Start: local[0],
		End:   local[len(local)-1],
	}, changedSince)
	if err != nil {
		return fmt.Errorf("sync: fetching flags for %q: %w", folder.Path, err)
	}

	if err := e.store.SetMessageFlags(ctx, folder.ID, updates); err != nil {
		return err
	}

	// Deletions are only computable from a complete answer. With CHANGEDSINCE
	// the server sent us a subset by design, and treating the messages missing
	// from it as deleted would wipe the whole folder.
	if changedSince != 0 {
		return nil
	}
	return e.store.DeleteMessagesByUID(ctx, folder.ID, missingUIDs(local, updates))
}

// condStoreSince decides whether this pass can ask the server for only what
// changed, and returns the modification sequence to ask from. Zero means
// "describe the whole window".
//
// CONDSTORE is only safe here when nothing was expunged, because an expunge
// does not advance HIGHESTMODSEQ — RFC 4551 is explicit about it. So the
// message count is the tell: if the mailbox holds exactly what it held plus
// what just arrived, nothing was removed, and the cheap query is correct.
// Any other number and we rescan the window to find out what went missing.
//
// A folder with no recorded count or modseq gets the full scan too. That is
// the first pass after an upgrade, and guessing there would be guessing about
// the user's mail.
func condStoreSince(caps imapx.Capabilities, folder model.Folder, sel imapx.SelectResult, arrived int) uint64 {
	if !caps.CondStore || folder.HighestModSeq == 0 || folder.TotalCount == 0 {
		return 0
	}
	if int(sel.NumMessages) != folder.TotalCount+arrived {
		return 0
	}
	return folder.HighestModSeq
}

// missingUIDs returns the UIDs we hold that the server did not mention.
//
// Only meaningful when the fetch asked for the complete range; see the caller.
func missingUIDs(local []uint32, present []model.FlagUpdate) []uint32 {
	alive := make(map[uint32]struct{}, len(present))
	for _, u := range present {
		alive[u.UID] = struct{}{}
	}

	var gone []uint32
	for _, uid := range local {
		if _, ok := alive[uid]; !ok {
			gone = append(gone, uid)
		}
	}
	return gone
}
