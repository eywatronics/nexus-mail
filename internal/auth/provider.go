package auth

import (
	"context"

	"github.com/emersion/go-sasl"

	"nexusmail/internal/model"
)

// CredentialProvider hands the IMAP layer a ready SASL client.
//
// This is the seam that keeps three authentication paths from becoming three
// IMAP code paths: imapx never learns whether it is talking to Gmail, Exchange
// or a generic server, because every provider hides how the credential was
// obtained.
type CredentialProvider interface {
	// SASLClient returns a client for a single authentication attempt. OAuth
	// providers mint a fresh access token here when the cached one has expired.
	SASLClient(ctx context.Context) (sasl.Client, error)

	// Refresh renews the credential if the provider supports it. Password
	// providers return nil without doing anything.
	Refresh(ctx context.Context) error

	Kind() model.AuthKind
}
