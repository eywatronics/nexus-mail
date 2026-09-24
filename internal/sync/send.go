package sync

import (
	"context"
	"errors"
	"fmt"

	"nexusmail/internal/model"
	"nexusmail/internal/smtpx"
)

// SenderDialer opens an authenticated submission connection for one account.
//
// A second dialer beside the IMAP one rather than a method on the first.
// Sending and receiving are separate services with separate hosts, separate
// ports and — on a corporate server — separate permissions; a client that
// could read mail would otherwise assume it could send it.
type SenderDialer func(ctx context.Context, accountID int64) (smtpx.MailSender, error)

// Outbox is where the bytes of a queued message live.
//
// An interface because the engine must not know that they are files: the queue
// row holds a name, and what a name means is the store's business.
type Outbox interface {
	Get(name string) ([]byte, error)
	Remove(name string) error
}

// SetSender gives the engine a way to submit mail.
//
// Optional, and that is deliberate. An engine with no sender still syncs; it
// simply fails any send it is asked to perform, with an error that says the
// account has no submission server rather than a nil dereference three frames
// down. That is the honest state of an account configured for IMAP only.
func (e *Engine) SetSender(dial SenderDialer, outbox Outbox) {
	e.sendMu.Lock()
	defer e.sendMu.Unlock()
	e.sendDial = dial
	e.outbox = outbox
}

func (e *Engine) sender() (SenderDialer, Outbox) {
	e.sendMu.RLock()
	defer e.sendMu.RUnlock()
	return e.sendDial, e.outbox
}

// partitionSends splits the queue into messages to submit and everything else.
//
// Split before dialing, not during. Every other operation needs an IMAP
// connection and a send does not, so an account whose queue holds only a
// message to send must not open a mailbox to discover that.
func partitionSends(ops []model.Operation) (sends, rest []model.Operation) {
	for _, op := range ops {
		if op.Kind == model.OpSend {
			sends = append(sends, op)
			continue
		}
		rest = append(rest, op)
	}
	return sends, rest
}

// drainSends submits queued messages over one connection.
//
// One connection for the batch: a submission server charges a TLS handshake
// and an authentication round trip per connection, and three messages written
// on a train are three messages, not three sessions.
func (e *Engine) drainSends(ctx context.Context, acct model.Account, ops []model.Operation) error {
	if len(ops) == 0 {
		return nil
	}

	dial, outbox := e.sender()
	if dial == nil || outbox == nil {
		// Recorded against each operation rather than returned. A missing
		// sender is a configuration problem the user has to fix, and failing
		// the whole drain would stop the flag changes and moves queued behind
		// it from reaching the server for as long as it went unfixed.
		for _, op := range ops {
			if err := e.failSend(ctx, op, errors.New(
				"sync: this account has no submission server configured")); err != nil {
				return err
			}
		}
		return nil
	}

	sender, err := dial(ctx, acct.ID)
	if err != nil {
		// A dial failure is transient by default: the server may be down, the
		// laptop may be on a train. Each operation is retried on its own
		// schedule rather than the batch being abandoned.
		for _, op := range ops {
			if err := e.retrySend(ctx, op,
				fmt.Errorf("sync: connecting to submit mail: %w", err)); err != nil {
				return err
			}
		}
		return nil
	}
	defer func() { _ = sender.Close() }()

	for _, op := range ops {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.submit(ctx, sender, outbox, op); err != nil {
			return err
		}
	}
	return nil
}

// submit sends one message and records what happened to it.
//
// The returned error is about the bookkeeping, never about the message. A
// server that refuses this one is recorded against the operation and the drain
// carries on; a database that cannot record the outcome stops it, because the
// next message would hit the same wall and the operation just sent would stay
// pending — and a pending send is one the worker will submit again.
func (e *Engine) submit(ctx context.Context, sender smtpx.MailSender, outbox Outbox,
	op model.Operation) error {

	raw, err := outbox.Get(op.Outbox)
	if err != nil {
		// The bytes are gone and no retry will bring them back. Anything else
		// would leave an operation failing forever against a file that does
		// not exist.
		return e.failSend(ctx, op,
			fmt.Errorf("the message is no longer in the outbox: %w", err))
	}

	env := smtpx.Envelope{From: op.EnvelopeFrom, To: op.EnvelopeTo}
	if err := sender.Send(ctx, env, raw); err != nil {
		if smtpx.IsPermanent(err) {
			// The file stays. A message the server refused is one the user may
			// want to read, fix and send again, and deleting it here would
			// throw away the only copy in existence.
			return e.failSend(ctx, op, err)
		}
		return e.retrySend(ctx, op, err)
	}

	// Done first, then the file. The other order risks a crash between them
	// leaving a queued operation whose message is gone — which the worker
	// would then mark permanently failed for a message that was in fact
	// delivered.
	//
	// The window is not closed, only made as small as one statement. If this
	// write fails the message has gone out and the queue still says pending,
	// so the next pass sends it again. Closing it properly needs the send and
	// the record to commit together, which SMTP does not offer; the honest
	// mitigation is to stop the drain here rather than send the rest of the
	// batch into the same fault.
	if err := e.store.MarkOperationDone(ctx, op.ID); err != nil {
		return fmt.Errorf("sync: the message was sent but could not be marked done: %w", err)
	}
	_ = outbox.Remove(op.Outbox)
	return nil
}

func (e *Engine) retrySend(ctx context.Context, op model.Operation, cause error) error {
	return e.store.MarkOperationFailed(ctx, op.ID, cause.Error(), queueRetryAt(op.Attempts))
}

func (e *Engine) failSend(ctx context.Context, op model.Operation, cause error) error {
	return e.store.MarkOperationPermanentlyFailed(ctx, op.ID, cause.Error())
}
