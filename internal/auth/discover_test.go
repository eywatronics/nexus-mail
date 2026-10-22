package auth

import (
	"context"
	"net"
	"strings"
	"testing"

	"nexusmail/internal/model"
)

type fakeResolver struct {
	srv   map[string][]*net.SRV
	mx    map[string][]*net.MX
	hosts map[string][]string
}

func (f fakeResolver) LookupSRV(_ context.Context, service, proto, name string) (string, []*net.SRV, error) {
	key := "_" + service + "._" + proto + "." + name
	if records, ok := f.srv[key]; ok {
		return "", records, nil
	}
	return "", nil, &net.DNSError{Err: "no such record", Name: key, IsNotFound: true}
}

func (f fakeResolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if records, ok := f.mx[name]; ok {
		return records, nil
	}
	return nil, &net.DNSError{Err: "no such record", Name: name, IsNotFound: true}
}

func (f fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	if addrs, ok := f.hosts[host]; ok {
		return addrs, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func TestDiscoverUsesPublishedSRVRecords(t *testing.T) {
	resolver := fakeResolver{srv: map[string][]*net.SRV{
		"_imaps._tcp.example.com": {
			{Target: "mail1.example.com.", Port: 993, Priority: 20, Weight: 0},
			{Target: "mail2.example.com.", Port: 993, Priority: 10, Weight: 0},
		},
		"_submissions._tcp.example.com": {
			{Target: "smtp.example.com.", Port: 587},
		},
	}}

	got, err := Discover(context.Background(), "ali@example.com", resolver)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if got.IMAPHost != "mail2.example.com" || got.IMAPPort != 993 {
		t.Errorf("IMAP = %s:%d, want the lowest-priority record mail2:993", got.IMAPHost, got.IMAPPort)
	}
	if got.Security != model.SecurityTLS {
		t.Errorf("Security = %q, want TLS for imaps", got.Security)
	}
	if got.SMTPHost != "smtp.example.com" || got.SMTPPort != 587 {
		t.Errorf("SMTP = %s:%d, want smtp.example.com:587", got.SMTPHost, got.SMTPPort)
	}
	if got.Source != "srv" {
		t.Errorf("Source = %q, want srv", got.Source)
	}
}

func TestDiscoverTreatsPlainIMAPSRVAsStartTLS(t *testing.T) {
	resolver := fakeResolver{srv: map[string][]*net.SRV{
		"_imap._tcp.example.com": {{Target: "imap.example.com.", Port: 143}},
	}}

	got, err := Discover(context.Background(), "ali@example.com", resolver)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if got.Security != model.SecuritySTARTTLS || got.IMAPPort != 143 {
		t.Errorf("got %s:%d %q, want 143 STARTTLS", got.IMAPHost, got.IMAPPort, got.Security)
	}
}

// A domain with MX records but no SRV is the normal corporate case. The MX
// itself is not an IMAP server, so the code only tests the conventional names
// and labels the answer as the guess it is.
func TestDiscoverFallsBackToAConventionalHost(t *testing.T) {
	resolver := fakeResolver{
		mx: map[string][]*net.MX{
			"example.com": {{Host: "mx1.example.com.", Pref: 10}},
		},
		hosts: map[string][]string{
			"imap.example.com": {"192.0.2.1"},
			"smtp.example.com": {"192.0.2.2"},
		},
	}

	got, err := Discover(context.Background(), "ali@example.com", resolver)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if got.IMAPHost != "imap.example.com" || got.IMAPPort != 993 {
		t.Errorf("IMAP = %s:%d, want the conventional imap.example.com:993", got.IMAPHost, got.IMAPPort)
	}
	if got.Source != "guess" {
		t.Errorf("Source = %q, want guess", got.Source)
	}
	if got.SMTPHost != "smtp.example.com" || got.SMTPPort != 587 {
		t.Errorf("SMTP = %s:%d, want smtp.example.com:587", got.SMTPHost, got.SMTPPort)
	}
}

func TestDiscoverReportsWhenThereIsNothingToFind(t *testing.T) {
	resolver := fakeResolver{mx: map[string][]*net.MX{
		"example.com": {{Host: "mx1.example.com.", Pref: 10}},
	}}

	_, err := Discover(context.Background(), "ali@example.com", resolver)
	if err == nil {
		t.Fatal("Discover() invented settings for a domain with no SRV and no conventional host")
	}
	if !strings.Contains(err.Error(), "ask your provider") {
		t.Errorf("error = %q; it should tell the person what to do next", err)
	}
}

func TestDiscoverRejectsAMalformedAddress(t *testing.T) {
	for _, email := range []string{"", "ali", "ali@", "@example.com"} {
		if _, err := Discover(context.Background(), email, fakeResolver{}); err == nil {
			t.Errorf("Discover(%q) accepted a malformed address", email)
		}
	}
}
