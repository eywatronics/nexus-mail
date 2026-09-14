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

// PasswordCredential is implemented by providers whose secret is a password,
// as opposed to a minted token.
//
// It exists because not every server offers the same way in. A password can be
// presented as SASL PLAIN, as SASL LOGIN, or through the IMAP LOGIN command,
// and which of those is available is the server's decision — on-premises
// Exchange in particular advertises different mechanisms depending on how its
// IMAP4 service is configured. Producing a sasl.Client is not enough when the
// mechanism cannot be chosen until the greeting has been read.
//
// Deliberately a second, narrow interface rather than a method on
// CredentialProvider: an OAuth provider has no password and should not be made
// to pretend otherwise.
type PasswordCredential interface {
	// Password returns the secret. Only ever called on an encrypted
	// connection; imapx refuses to reach this point on a cleartext one.
	Password(ctx context.Context) (string, error)
	// Username is the identity the password belongs to.
	Username() string
}
