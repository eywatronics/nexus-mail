package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"

	"nexusmail/internal/model"
)

// tokenEndpoint serves a token response and records what was asked for.
type tokenEndpoint struct {
	srv *httptest.Server

	hits        atomic.Int64
	gotGrant    string
	gotRefresh  string
	gotClientID string
}

func newTokenEndpoint(t *testing.T, accessToken, rotatedRefresh string) *tokenEndpoint {
	t.Helper()

	e := &tokenEndpoint{}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.hits.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm(): %v", err)
		}
		e.gotGrant = r.Form.Get("grant_type")
		e.gotRefresh = r.Form.Get("refresh_token")
		e.gotClientID = r.Form.Get("client_id")

		body := `{"access_token":"` + accessToken + `","token_type":"Bearer","expires_in":3600`
		if rotatedRefresh != "" {
			body += `,"refresh_token":"` + rotatedRefresh + `"`
		}
		body += `}`

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write token response: %v", err)
		}
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func TestOAuthProviderExchangesStoredRefreshTokenForXOAUTH2Client(t *testing.T) {
	endpoint := newTokenEndpoint(t, "fresh-at", "")

	store := testStore(t)
	if err := store.Set("account:u@example.com", "stored-rt"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewOAuthProvider("u@example.com", "account:u@example.com", store, OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: endpoint.srv.URL},
	})
	if p.Kind() != model.AuthOAuth {
		t.Errorf("Kind() = %q, want %q", p.Kind(), model.AuthOAuth)
	}

	client, err := p.SASLClient(context.Background())
	if err != nil {
		t.Fatalf("SASLClient() error: %v", err)
	}
	if endpoint.gotGrant != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", endpoint.gotGrant)
	}
	if endpoint.gotRefresh != "stored-rt" {
		t.Errorf("refresh_token = %q, want the value from the SecretStore", endpoint.gotRefresh)
	}

	mech, ir, err := client.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if mech != "XOAUTH2" {
		t.Errorf("mechanism = %q, want XOAUTH2", mech)
	}
	if !strings.Contains(string(ir), "auth=Bearer fresh-at") {
		t.Errorf("initial response %q does not carry the freshly minted access token", ir)
	}
	if !strings.Contains(string(ir), "user=u@example.com") {
		t.Errorf("initial response %q does not carry the username", ir)
	}
}

// Providers rotate refresh tokens. Dropping a rotated value kills the account
// silently once the previous one expires — days later, with no obvious cause.
func TestOAuthProviderPersistsRotatedRefreshToken(t *testing.T) {
	endpoint := newTokenEndpoint(t, "at", "rotated-rt")

	store := testStore(t)
	if err := store.Set("ref", "original-rt"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewOAuthProvider("u@example.com", "ref", store, OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: endpoint.srv.URL},
	})
	if _, err := p.SASLClient(context.Background()); err != nil {
		t.Fatalf("SASLClient() error: %v", err)
	}

	got, err := store.Get("ref")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "rotated-rt" {
		t.Errorf("stored refresh token = %q, want rotated-rt", got)
	}
}

// The token endpoint is rate-limited by providers, and the sync engine opens
// two connections per account. Minting a token per connection would double the
// requests for no benefit.
func TestOAuthProviderCachesTheAccessToken(t *testing.T) {
	endpoint := newTokenEndpoint(t, "at", "")

	store := testStore(t)
	if err := store.Set("ref", "rt"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewOAuthProvider("u@example.com", "ref", store, OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: endpoint.srv.URL},
	})
	for i := range 3 {
		if _, err := p.SASLClient(context.Background()); err != nil {
			t.Fatalf("SASLClient() call %d error: %v", i+1, err)
		}
	}
	if got := endpoint.hits.Load(); got != 1 {
		t.Errorf("token endpoint hit %d times for three connections, want 1", got)
	}
}

// Refresh exists so the sync engine can retry once before declaring an account
// signed out. That only helps if it actually bypasses the cache.
func TestOAuthProviderRefreshBypassesTheCache(t *testing.T) {
	endpoint := newTokenEndpoint(t, "at", "")

	store := testStore(t)
	if err := store.Set("ref", "rt"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewOAuthProvider("u@example.com", "ref", store, OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: endpoint.srv.URL},
	})
	if _, err := p.SASLClient(context.Background()); err != nil {
		t.Fatalf("SASLClient() error: %v", err)
	}
	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}
	if got := endpoint.hits.Load(); got != 2 {
		t.Errorf("token endpoint hit %d times, want 2 (one cached mint plus one forced refresh)", got)
	}
}

func TestOAuthProviderReportsAMissingRefreshToken(t *testing.T) {
	endpoint := newTokenEndpoint(t, "at", "")

	p := NewOAuthProvider("u@example.com", "account:absent", testStore(t), OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: endpoint.srv.URL},
	})
	_, err := p.SASLClient(context.Background())
	if err == nil {
		t.Fatal("SASLClient() succeeded with no stored refresh token")
	}
	if endpoint.hits.Load() != 0 {
		t.Error("the token endpoint was called without a refresh token to send")
	}
}

func TestOAuthProviderSurfacesAnInvalidGrant(t *testing.T) {
	// invalid_grant is what a revoked or expired refresh token returns, and it
	// is the signal that the user has to sign in again rather than the app
	// retrying forever.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error":"invalid_grant"}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	store := testStore(t)
	if err := store.Set("ref", "revoked-rt"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewOAuthProvider("u@example.com", "ref", store, OAuthConfig{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	})
	_, err := p.SASLClient(context.Background())
	if err == nil {
		t.Fatal("SASLClient() ignored invalid_grant")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error %q loses the provider's reason, which the sync engine classifies on", err)
	}
}

func TestEndpointsAndScopesMatchProviderDocumentation(t *testing.T) {
	// These strings are not guessable and a typo produces a confusing runtime
	// failure, so they are pinned here against the providers' documentation.
	if got := MicrosoftEndpoint().AuthURL; !strings.Contains(got, "login.microsoftonline.com/common") {
		t.Errorf("Microsoft AuthURL = %q, want the common authority", got)
	}
	if got := GoogleEndpoint().TokenURL; got != "https://oauth2.googleapis.com/token" {
		t.Errorf("Google TokenURL = %q", got)
	}

	ms := strings.Join(MicrosoftScopes(), " ")
	for _, want := range []string{
		"https://outlook.office.com/IMAP.AccessAsUser.All",
		"https://outlook.office.com/SMTP.Send",
		// Without offline_access the account stops working an hour after
		// sign-in, because no refresh token is issued.
		"offline_access",
	} {
		if !strings.Contains(ms, want) {
			t.Errorf("Microsoft scopes %q are missing %q", ms, want)
		}
	}

	google := strings.Join(GoogleScopes(), " ")
	if !strings.Contains(google, "https://mail.google.com/") {
		t.Errorf("Google scopes %q are missing full IMAP access", google)
	}
}
