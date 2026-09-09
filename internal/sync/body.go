package sync

import (
	"context"
	"fmt"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// EnsureBody returns a message body, fetching it from the server only when it
// is not already cached.
//
// Once cached, the message reads with no network at all — which is the
// local-first promise in one function. A test proves it by blanking the fake
// server's bodies before the second call.
func (e *Engine) EnsureBody(ctx context.Context, acct model.Account, folder model.Folder, msg model.Message) (imapx.Body, error) {
	html, text, err := e.store.GetMessageBody(ctx, msg.ID)
	if err != nil {
		return imapx.Body{}, fmt.Errorf("sync: reading cached body: %w", err)
	}
	if html != "" || text != "" {
		return imapx.Body{HTML: html, Text: text}, nil
	}

	be, err := e.dial(ctx, acct.ID)
	if err != nil {
		return imapx.Body{}, fmt.Errorf("sync: connecting account %d: %w", acct.ID, err)
	}
	// The body lives in a mailbox, and IMAP fetches are scoped to the selected
	// one. Selecting here rather than assuming the caller did keeps EnsureBody
	// usable from anywhere.
	if _, err := be.Select(ctx, folder.Path); err != nil {
		return imapx.Body{}, fmt.Errorf("sync: selecting %q: %w", folder.Path, err)
	}

	body, err := be.FetchBody(ctx, msg.UID)
	if err != nil {
		return imapx.Body{}, fmt.Errorf("sync: fetching body for UID %d: %w", msg.UID, err)
	}
	if err := e.store.SetMessageBody(ctx, msg.ID, body.HTML, body.Text); err != nil {
		return imapx.Body{}, fmt.Errorf("sync: caching body: %w", err)
	}
	return body, nil
}
