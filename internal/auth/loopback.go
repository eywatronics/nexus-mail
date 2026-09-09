package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"

	"golang.org/x/oauth2"
)

// OAuthConfig describes one provider's authorization-code flow.
type OAuthConfig struct {
	ClientID string
	// ClientSecret is empty for public clients, which is what a desktop app
	// is: it cannot keep a secret on the user's machine.
	ClientSecret string
	Scopes       []string
	Endpoint     oauth2.Endpoint

	// RedirectPort forces a fixed loopback port. Zero, the default, picks a
	// random one — correct for both Google desktop clients and Entra native
	// clients, which accept any loopback port. Set it only where security
	// software blocks binding arbitrary ports, and register the same port with
	// the provider.
	RedirectPort int

	// OpenBrowser launches the system browser. Tests substitute their own.
	OpenBrowser func(url string) error
}

// RunLoopbackFlow performs RFC 8252 authorization on a loopback redirect with
// PKCE, blocking until the user finishes in their browser or ctx expires.
//
// The listener binds an ephemeral port rather than a fixed one. A fixed port
// already in use would break sign-in with no recovery path the user could
// discover.
func RunLoopbackFlow(ctx context.Context, cfg OAuthConfig) (*oauth2.Token, error) {
	if cfg.ClientID == "" {
		return nil, errors.New("auth: OAuth client ID is required; see docs/oauth-setup.md")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(cfg.RedirectPort))
	if err != nil {
		if cfg.RedirectPort != 0 {
			return nil, fmt.Errorf(
				"auth: cannot bind the configured redirect port %d: %w", cfg.RedirectPort, err)
		}
		return nil, fmt.Errorf("auth: cannot open loopback listener: %w", err)
	}
	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		return nil, err
	}

	conf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       cfg.Scopes,
		Endpoint:     cfg.Endpoint,
		RedirectURL:  redirectURI,
	}

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()

			deliver := func(res result, message string) {
				writeBrowserPage(w, message)
				// Buffered channel with the first write winning: a stray
				// second request must not block the handler.
				select {
				case results <- res:
				default:
				}
			}

			if providerErr := q.Get("error"); providerErr != "" {
				deliver(result{err: fmt.Errorf("auth: provider returned error %q: %s",
					providerErr, q.Get("error_description"))},
					"Sign-in failed. You can close this window.")
				return
			}
			// The state check runs before the code is read, so a forged
			// response never reaches the token endpoint.
			if q.Get("state") != state {
				deliver(result{err: errors.New("auth: state mismatch; discarding the response")},
					"Sign-in failed. You can close this window.")
				return
			}
			code := q.Get("code")
			if code == "" {
				deliver(result{err: errors.New("auth: provider returned no authorization code")},
					"Sign-in failed. You can close this window.")
				return
			}
			deliver(result{code: code},
				"Signed in. You can close this window and return to Nexus Mail.")
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	authURL := conf.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	)

	open := cfg.OpenBrowser
	if open == nil {
		open = openSystemBrowser
	}
	if err := open(authURL); err != nil {
		return nil, fmt.Errorf("auth: cannot open browser: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-results:
		if res.err != nil {
			return nil, res.err
		}
		tok, err := conf.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
		if err != nil {
			return nil, fmt.Errorf("auth: token exchange failed: %w", err)
		}
		return tok, nil
	}
}

func writeBrowserPage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w,
		"<!doctype html><meta charset=utf-8><title>Nexus Mail</title>"+
			"<body style=\"font:16px system-ui;padding:3rem;text-align:center\"><p>%s</p>", msg)
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openSystemBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
