package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nexusmail/internal/model"
)

func testStore(t *testing.T) SecretStore {
	t.Helper()
	s, err := NewFileStore(t.TempDir(), "test-master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	return s
}

// The wire format is taken verbatim from Microsoft's documentation, including
// their example account and token, so a mistake in the encoding shows up here
// rather than as an opaque server rejection.
func TestXOAUTH2InitialResponseFormat(t *testing.T) {
	c := NewXOAUTH2Client(
		"test@contoso.onmicrosoft.com",
		"EwBAAl3BAAUFFpUAo7J3Ve0bjLBWZWCclRC3EoAA",
	)

	mech, ir, err := c.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if mech != "XOAUTH2" {
		t.Errorf("mechanism = %q, want XOAUTH2", mech)
	}
	want := "user=test@contoso.onmicrosoft.com\x01" +
		"auth=Bearer EwBAAl3BAAUFFpUAo7J3Ve0bjLBWZWCclRC3EoAA\x01\x01"
	if string(ir) != want {
		t.Errorf("initial response = %q, want %q", ir, want)
	}
}

// A server that rejects the token replies with a base64 JSON error challenge
// and expects an empty client response before it sends the tagged NO. We must
// surface that as an error rather than hanging or returning a blank success.
func TestXOAUTH2SurfacesServerChallengeAsError(t *testing.T) {
	c := NewXOAUTH2Client("user@example.com", "expired-token")
	if _, _, err := c.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	const challenge = `{"status":"400","schemes":"Bearer","scope":"https://mail.google.com/"}`
	resp, err := c.Next([]byte(challenge))
	if err == nil {
		t.Fatal("Next() returned no error for a server error challenge")
	}
	if len(resp) != 0 {
		t.Errorf("Next() response = %q, want empty", resp)
	}
	// The challenge names the missing scope, which is exactly what a
	// misconfigured OAuth client needs to be told.
	if !strings.Contains(err.Error(), "mail.google.com") {
		t.Errorf("error %q drops the server's explanation", err)
	}
}

func TestPasswordProviderReturnsPlainClientWithStoredSecret(t *testing.T) {
	store := testStore(t)
	if err := store.Set("account:u@example.com", "app-password"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	p := NewPasswordProvider("u@example.com", "account:u@example.com", store)
	if p.Kind() != model.AuthPassword {
		t.Errorf("Kind() = %q, want %q", p.Kind(), model.AuthPassword)
	}

	client, err := p.SASLClient(context.Background())
	if err != nil {
		t.Fatalf("SASLClient() error: %v", err)
	}
	mech, ir, err := client.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if mech != "PLAIN" {
		t.Errorf("mechanism = %q, want PLAIN", mech)
	}
	want := "\x00u@example.com\x00app-password"
	if string(ir) != want {
		t.Errorf("initial response = %q, want %q", ir, want)
	}
}

func TestPasswordProviderReportsMissingSecret(t *testing.T) {
	p := NewPasswordProvider("u@example.com", "account:missing", testStore(t))
	if _, err := p.SASLClient(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Errorf("SASLClient() error = %v, want ErrNotFound", err)
	}
}

func TestPasswordProviderRefreshIsANoOp(t *testing.T) {
	// A password does not expire on its own; if it stops working the user
	// changed it, and retrying cannot help.
	p := NewPasswordProvider("u@example.com", "ref", testStore(t))
	if err := p.Refresh(context.Background()); err != nil {
		t.Errorf("Refresh() = %v, want nil", err)
	}
}

func TestPresetForKnownDomains(t *testing.T) {
	cases := []struct {
		email        string
		wantHost     string
		wantPort     int
		wantProvider model.Provider
	}{
		{"a@gmail.com", "imap.gmail.com", 993, model.ProviderGoogle},
		{"a@googlemail.com", "imap.gmail.com", 993, model.ProviderGoogle},
		{"a@outlook.com", "outlook.office365.com", 993, model.ProviderMicrosoft},
		{"a@hotmail.com", "outlook.office365.com", 993, model.ProviderMicrosoft},
		{"a@yandex.com.tr", "imap.yandex.com.tr", 993, model.ProviderGeneric},
		// Case must not matter: users type their address however they like.
		{"A@GMAIL.COM", "imap.gmail.com", 993, model.ProviderGoogle},
		// A plus-address is still the same domain.
		{"a+tag@gmail.com", "imap.gmail.com", 993, model.ProviderGoogle},
	}
	for _, tc := range cases {
		got, ok := PresetFor(tc.email)
		if !ok {
			t.Errorf("PresetFor(%q) reported no preset", tc.email)
			continue
		}
		if got.IMAPHost != tc.wantHost {
			t.Errorf("PresetFor(%q).IMAPHost = %q, want %q", tc.email, got.IMAPHost, tc.wantHost)
		}
		if got.IMAPPort != tc.wantPort {
			t.Errorf("PresetFor(%q).IMAPPort = %d, want %d", tc.email, got.IMAPPort, tc.wantPort)
		}
		if got.Provider != tc.wantProvider {
			t.Errorf("PresetFor(%q).Provider = %q, want %q", tc.email, got.Provider, tc.wantProvider)
		}
		if got.SMTPHost == "" || got.SMTPPort == 0 {
			t.Errorf("PresetFor(%q) has no SMTP settings; P2 will need them", tc.email)
		}
	}
}

func TestPresetForUnknownDomain(t *testing.T) {
	// A corporate domain is the common case here: it cannot be guessed, only
	// asked for. Claiming a preset would send the user to the wrong server.
	if _, ok := PresetFor("someone@ozdilek.com.tr"); ok {
		t.Error("PresetFor() claimed a preset for a corporate domain")
	}
	if _, ok := PresetFor("someone@self-hosted.example"); ok {
		t.Error("PresetFor() claimed a preset for an unknown domain")
	}
}

func TestPresetForMalformedAddress(t *testing.T) {
	for _, email := range []string{"", "no-at-sign", "@nodomain", "trailing@"} {
		if _, ok := PresetFor(email); ok {
			t.Errorf("PresetFor(%q) returned a preset for a malformed address", email)
		}
	}
}
