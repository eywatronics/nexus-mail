package auth

import (
	"context"

	"github.com/emersion/go-sasl"

	"nexusmail/internal/model"
)

type passwordProvider struct {
	username  string
	secretRef string
	store     SecretStore
}

// NewPasswordProvider authenticates with SASL PLAIN using a password or app
// password read from the SecretStore.
//
// PLAIN sends the credential in the clear, so callers must only use this over
// TLS. imapx enforces that for production connections; tests are the only
// place a plaintext connection is allowed.
func NewPasswordProvider(username, secretRef string, store SecretStore) CredentialProvider {
	return &passwordProvider{username: username, secretRef: secretRef, store: store}
}

func (p *passwordProvider) SASLClient(_ context.Context) (sasl.Client, error) {
	secret, err := p.store.Get(p.secretRef)
	if err != nil {
		return nil, err
	}
	return sasl.NewPlainClient("", p.username, secret), nil
}

// Refresh is a no-op: a password does not expire on its own. If it stops
// working the user changed it, which no amount of retrying will fix.
func (p *passwordProvider) Refresh(_ context.Context) error { return nil }

func (p *passwordProvider) Kind() model.AuthKind { return model.AuthPassword }
