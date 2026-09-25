package sync

import (
	"context"
	"fmt"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// queueBatchSize bounds one drain. The queue is drained again on every pass,
// so a large backlog moves in chunks rather than holding the connection for
// minutes before the first new message can arrive.
const queueBatchSize = 100

// queueRetryBase is the first wait after a failed operation. It doubles per
// attempt up to the connection backoff cap.
const queueRetryBase = 30 * time.Second

// DrainQueue sends an account's pending changes to the server.
func (e *Engine) DrainQueue(ctx context.Context, acct model.Account) error {
	ops, err := e.store.ClaimOperations(ctx, acct.ID, queueBatchSize)
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		// The common case. Dialing to discover there is nothing to do would
		// open a connection on every pass of an idle account.
		return nil
	}

	// Messages to submit go over SMTP and everything else over IMAP, so the
	// two are separated before either connection is opened. An account whose
	// queue holds only a message written offline must not open a mailbox to
	// find that out — and one holding only flag changes must not open a
	// submission connection.
	sends, rest := partitionSends(ops)
	if err := e.drainSends(ctx, acct, sends); err != nil {
		return err
	}
	if len(rest) == 0 {
		return nil
	}

	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return fmt.Errorf("sync: connecting account %d to drain the queue: %w", acct.ID, err)
	}
	defer func() { _ = be.Close() }()

	return e.drainOn(ctx, be, acct, rest)
}

// drainOn applies claimed operations over a connection the caller already has.
func (e *Engine) drainOn(ctx context.Context, be imapx.MailBackend, acct model.Account, ops []model.Operation) error {
	folders, err := e.store.ListFolders(ctx, acct.ID)
	if err != nil {
		return err
	}
	byID := map[int64]model.Folder{}
	for _, f := range folders {
		byID[f.ID] = f
	}

	for _, op := range ops {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.applyOperation(ctx, be, op, byID); err != nil {
			return err
		}
	}
	return nil
}

// applyOperation sends one operation and records what happened to it.
//
// It returns an error only for problems with the queue itself. A server that
// refuses the command is recorded against the operation and the drain carries
// on: one unusable change must not freeze every other change on the account.
func (e *Engine) applyOperation(ctx context.Context, be imapx.MailBackend, op model.Operation, folders map[int64]model.Folder) error {
	folder, ok := folders[op.FolderID]
	if !ok {
		// The folder is gone from the account entirely. Nothing to apply the
		// operation to, and no reason to keep retrying it.
		return e.store.MarkOperationDropped(ctx, op.ID)
	}

	// The UIDVALIDITY stamp, first pass. A mismatch against what we have
	// stored is already enough to drop the operation, and it saves selecting a
	// mailbox we are not going to touch.
	if op.UIDValidity != folder.UIDValidity {
		return e.store.MarkOperationDropped(ctx, op.ID)
	}

	sel, err := be.Select(ctx, folder.Path)
	if err != nil {
		return e.recordFailure(ctx, op, err)
	}

	// Second pass, against what the server says right now. This is the check
	// that actually matters: the stored value is only as fresh as the last
	// sync, and a mailbox recreated in the last few seconds — or by another
	// session entirely — would still look unchanged from here. Comparing at
	// the moment of applying is the only version with no window in it.
	//
	// Without this, the queued delete of UID 55 goes to a mailbox where 55 is
	// now somebody else's mail, and nobody ever finds out why it vanished.
	if op.UIDValidity != sel.UIDValidity {
		return e.store.MarkOperationDropped(ctx, op.ID)
	}

	if err := e.sendOperation(ctx, be, op, folders); err != nil {
		return e.recordFailure(ctx, op, err)
	}
	return e.store.MarkOperationDone(ctx, op.ID)
}

// sendOperation maps one queued row onto one IMAP command.
func (e *Engine) sendOperation(ctx context.Context, be imapx.MailBackend, op model.Operation, folders map[int64]model.Folder) error {
	switch op.Kind {
	case model.OpAddFlags:
		return be.StoreFlags(ctx, op.UIDs, op.Flags, true)
	case model.OpRemoveFlags:
		return be.StoreFlags(ctx, op.UIDs, op.Flags, false)
	case model.OpMove:
		dest, ok := folders[op.TargetFolderID]
		if !ok {
			return fmt.Errorf("the destination folder no longer exists")
		}
		return be.Move(ctx, op.UIDs, dest.Path)
	case model.OpDelete:
		return be.Expunge(ctx, op.UIDs)
	case model.OpEmptyFolder:
		return be.EmptyFolder(ctx)
	default:
		return fmt.Errorf("unknown operation kind %q", op.Kind)
	}
}

// recordFailure decides whether an operation is worth trying again.
//
// The split is the same one the sync engine already makes: a dropped
// connection fixes itself, a read-only mailbox does not. Retrying the second
// kind forever would keep an unfixable change in the queue and keep telling
// the user something is still in flight.
func (e *Engine) recordFailure(ctx context.Context, op model.Operation, cause error) error {
	// Auth failures are retried too: the credential provider refreshes the
	// token on the next connection, which often fixes it without the user
	// doing anything.
	if class := Classify(cause); class != ClassTransient && class != ClassAuth {
		return e.store.MarkOperationPermanentlyFailed(ctx, op.ID, cause.Error())
	}
	return e.store.MarkOperationFailed(ctx, op.ID, cause.Error(), queueRetryAt(op.Attempts))
}

// queueRetryAt spaces retries out the same way reconnects are spaced, and for
// the same reason: a server that just refused a command will refuse it again a
// second later.
func queueRetryAt(attempts int) time.Time {
	d := queueRetryBase
	for range attempts {
		d *= 2
		if d >= backoffCap {
			d = backoffCap
			break
		}
	}
	return time.Now().Add(d)
}
