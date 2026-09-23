package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeAuthServer stands in for Google or Microsoft, recording what the client
// sent so the test can assert on PKCE and redirect handling.
type fakeAuthServer struct {
	srv *httptest.Server

	gotChallenge string
	gotMethod    string
	gotRedirect  string
	gotVerifier  string
	gotClientID  string
	gotScope     string

	// tokenHits counts exchanges, so a test can assert the endpoint was never
	// reached.
	tokenHits int
}

func newFakeAuthServer(t *testing.T, refreshToken string) *fakeAuthServer {
	t.Helper()

	f := &fakeAuthServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.gotChallenge = q.Get("code_challenge")
		f.gotMethod = q.Get("code_challenge_method")
		f.gotRedirect = q.Get("redirect_uri")
		f.gotClientID = q.Get("client_id")
		f.gotScope = q.Get("scope")

		// Redirect back to the loopback listener the client opened, echoing
		// the state so the client can verify it.
		back := q.Get("redirect_uri") + "?code=test-auth-code&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, back, http.StatusFound)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.tokenHits++
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm(): %v", err)
		}
		f.gotVerifier = r.Form.Get("code_verifier")

		w.Header().Set("Content-Type", "application/json")
		body := `{"access_token":"at-1","token_type":"Bearer","expires_in":3600`
		if refreshToken != "" {
			body += `,"refresh_token":"` + refreshToken + `"`
		}
		body += `}`
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write token response: %v", err)
		}
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// fetchInBackground stands in for the system browser: it follows the
// authorization URL, and the fake server's redirect lands on the loopback
// listener, completing the flow exactly as a real browser would.
func fetchInBackground(t *testing.T, recordURL *string) func(string) error {
	t.Helper()
	return func(u string) error {
		if recordURL != nil {
			*recordURL = u
		}
		go func() {
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get(u)
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

func TestRunLoopbackFlowUsesPKCEAndRandomPort(t *testing.T) {
	fake := newFakeAuthServer(t, "rt-1")

	var openedURL string
	cfg := OAuthConfig{
		ClientID: "client-abc",
		Scopes:   []string{"https://mail.google.com/"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  fake.srv.URL + "/authorize",
			TokenURL: fake.srv.URL + "/token",
		},
		OpenBrowser: fetchInBackground(t, &openedURL),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tok, err := RunLoopbackFlow(ctx, cfg)
	if err != nil {
		t.Fatalf("RunLoopbackFlow() error: %v", err)
	}
	if tok.RefreshToken != "rt-1" {
		t.Errorf("RefreshToken = %q, want rt-1", tok.RefreshToken)
	}
	if tok.AccessToken != "at-1" {
		t.Errorf("AccessToken = %q, want at-1", tok.AccessToken)
	}

	if fake.gotMethod != "S256" {
		t.Errorf("code_challenge_method = %q, want S256; plain PKCE is not acceptable", fake.gotMethod)
	}
	if fake.gotVerifier == "" {
		t.Fatal("the token request carried no code_verifier; PKCE is not wired up")
	}
	// The challenge must be the S256 hash of the verifier that was later sent.
	// Asserting only that both fields are present would pass even if they were
	// unrelated.
	sum := sha256.Sum256([]byte(fake.gotVerifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if fake.gotChallenge != wantChallenge {
		t.Errorf("code_challenge = %q, want %q (S256 of the verifier)", fake.gotChallenge, wantChallenge)
	}

	u, err := url.Parse(fake.gotRedirect)
	if err != nil {
		t.Fatalf("parse redirect_uri %q: %v", fake.gotRedirect, err)
	}
	if u.Hostname() != "127.0.0.1" {
		t.Errorf("redirect host = %q, want 127.0.0.1", u.Hostname())
	}
	if u.Port() == "" || u.Port() == "0" {
		t.Errorf("redirect port = %q, want a concrete ephemeral port", u.Port())
	}
	if u.Port() == "3000" {
		t.Error("redirect uses the fixed port 3000; a busy port would break sign-in with no way out")
	}

	if fake.gotClientID != "client-abc" {
		t.Errorf("client_id = %q, want client-abc", fake.gotClientID)
	}
	if !strings.Contains(fake.gotScope, "mail.google.com") {
		t.Errorf("scope = %q, want it to carry the requested scope", fake.gotScope)
	}
	if !strings.Contains(openedURL, "client-abc") {
		t.Errorf("browser URL %q does not carry the client ID", openedURL)
	}
}

func TestRunLoopbackFlowRejectsStateMismatch(t *testing.T) {
	var tokenHits int
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		// Return the wrong state, as a forged response would.
		back := r.URL.Query().Get("redirect_uri") + "?code=evil&state=not-the-state-we-sent"
		http.Redirect(w, r, back, http.StatusFound)
	})
	mux.HandleFunc("/token", func(_ http.ResponseWriter, _ *http.Request) {
		tokenHits++
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := OAuthConfig{
		ClientID:    "c",
		Endpoint:    oauth2.Endpoint{AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/token"},
		OpenBrowser: fetchInBackground(t, nil),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := RunLoopbackFlow(ctx, cfg); err == nil {
		t.Fatal("RunLoopbackFlow() accepted a mismatched state")
	}
	// The state check must happen before the exchange, not after.
	if tokenHits != 0 {
		t.Errorf("the token endpoint was reached %d times after a state mismatch, want 0", tokenHits)
	}
}

func TestRunLoopbackFlowSurfacesProviderError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		back := r.URL.Query().Get("redirect_uri") +
			"?error=access_denied&error_description=The+user+declined"
		http.Redirect(w, r, back, http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := RunLoopbackFlow(ctx, OAuthConfig{
		ClientID:    "c",
		Endpoint:    oauth2.Endpoint{AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/token"},
		OpenBrowser: fetchInBackground(t, nil),
	})
	if err == nil {
		t.Fatal("RunLoopbackFlow() ignored the provider's error response")
	}
	// A user who declines consent should see why, not a generic timeout.
	if !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("error %q drops the provider's reason", err)
	}
}

func TestRunLoopbackFlowHonoursAFixedPort(t *testing.T) {
	fake := newFakeAuthServer(t, "rt-1")

	// Pick a free port the OS hands us, then ask the flow to bind that exact
	// one — mirroring a user who registered a specific port with the provider.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listener: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := RunLoopbackFlow(ctx, OAuthConfig{
		ClientID:     "c",
		RedirectPort: port,
		Endpoint: oauth2.Endpoint{
			AuthURL:  fake.srv.URL + "/authorize",
			TokenURL: fake.srv.URL + "/token",
		},
		OpenBrowser: fetchInBackground(t, nil),
	}); err != nil {
		t.Fatalf("RunLoopbackFlow() error: %v", err)
	}

	u, err := url.Parse(fake.gotRedirect)
	if err != nil {
		t.Fatalf("parse redirect_uri: %v", err)
	}
	if u.Port() != strconv.Itoa(port) {
		t.Errorf("redirect port = %q, want the configured %d", u.Port(), port)
	}
}

func TestRunLoopbackFlowRequiresAClientID(t *testing.T) {
	_, err := RunLoopbackFlow(context.Background(), OAuthConfig{})
	if err == nil {
		t.Fatal("RunLoopbackFlow() accepted an empty client ID")
	}
	// Open-source users have to register their own client, so the error has to
	// point at the guide rather than just complaining.
	if !strings.Contains(err.Error(), "oauth-setup") {
		t.Errorf("error %q does not point the user at the setup guide", err)
	}
}

func TestRunLoopbackFlowRespectsContextCancellation(t *testing.T) {
	mux := http.NewServeMux()
	// Never redirect back, so the flow can only end via the context.
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := RunLoopbackFlow(ctx, OAuthConfig{
		ClientID:    "c",
		Endpoint:    oauth2.Endpoint{AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/token"},
		OpenBrowser: fetchInBackground(t, nil),
	}); err == nil {
		t.Fatal("RunLoopbackFlow() did not stop when the context expired")
	}
	// A user who abandons sign-in must not leave a listener and a goroutine
	// alive for the rest of the session.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("flow took %v to honour a 300ms context", elapsed)
	}
}
