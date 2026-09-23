package auth

import (
	"context"
	"fmt"
	"sync"

	"github.com/emersion/go-sasl"
	"golang.org/x/oauth2"

	"nexusmail/internal/model"
)

type oauthProvider struct {
	mu        sync.Mutex
	username  string
	secretRef string
	store     SecretStore
	conf      *oauth2.Config
	// cached holds the last access token so repeated connections within its
	// lifetime do not hit the token endpoint again. Providers rate-limit that
	// endpoint, and a sync engine opening two connections per account would
	// otherwise mint two tokens every reconnect.
	cached *oauth2.Token
}

// NewOAuthProvider authenticates with XOAUTH2, minting access tokens from the
// refresh token held in the SecretStore.
func NewOAuthProvider(username, secretRef string, store SecretStore, cfg OAuthConfig) CredentialProvider {
	return &oauthProvider{
		username:  username,
		secretRef: secretRef,
		store:     store,
		conf: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Scopes:       cfg.Scopes,
			Endpoint:     cfg.Endpoint,
		},
	}
}

func (p *oauthProvider) SASLClient(ctx context.Context) (sasl.Client, error) {
	tok, err := p.token(ctx)
	if err != nil {
		return nil, err
	}
	return NewXOAUTH2Client(p.username, tok.AccessToken), nil
}

// Refresh discards the cached token and mints a new one. The sync engine calls
// this after an authentication failure, before deciding the account needs the
// user to sign in again.
func (p *oauthProvider) Refresh(ctx context.Context) error {
	p.mu.Lock()
	p.cached = nil
	p.mu.Unlock()

	_, err := p.token(ctx)
	return err
}

func (p *oauthProvider) Kind() model.AuthKind { return model.AuthOAuth }

func (p *oauthProvider) token(ctx context.Context) (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cached != nil && p.cached.Valid() {
		return p.cached, nil
	}

	refresh, err := p.store.Get(p.secretRef)
	if err != nil {
		return nil, fmt.Errorf("auth: cannot read refresh token: %w", err)
	}

	tok, err := p.conf.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh}).Token()
	if err != nil {
		return nil, fmt.Errorf("auth: refreshing access token failed: %w", err)
	}

	// Providers rotate refresh tokens. Dropping a rotated value would kill the
	// account silently once the previous one expired — days later, with no
	// obvious cause.
	if tok.RefreshToken != "" && tok.RefreshToken != refresh {
		if err := p.store.Set(p.secretRef, tok.RefreshToken); err != nil {
			return nil, fmt.Errorf("auth: cannot persist rotated refresh token: %w", err)
		}
	}

	p.cached = tok
	return tok, nil
}
