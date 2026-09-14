package imapx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2/imapclient"

	"nexusmail/internal/model"
)

// dialWithCA runs the real connect path with the test authority trusted, so a
// genuine TLS handshake happens against a certificate that is genuinely
// verified — just not by a public root. connect takes the client options, so
// this needs no knob on the public Config.
func dialWithCA(t *testing.T, addr string, cfg Config, pool *x509.CertPool) (*imapclient.Client, error) {
	t.Helper()
	return connect(addr, cfg, &imapclient.Options{
		TLSConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	})
}

// On-premises Exchange is the reason STARTTLS exists here. Its IMAP4 service
// defaults to LoginType SecureLogin, which will not take a password until the
// connection has been upgraded, and plenty of deployments never open 993 at
// all — so without this the server is simply unreachable.
func TestSTARTTLSUpgradesTheConnection(t *testing.T) {
	serverTLS, pool := ownCA(t)
	addr, _ := startSTARTTLSServer(t, serverTLS)

	cfg := configFor(t, addr)
	cfg.Insecure = false
	cfg.Security = model.SecuritySTARTTLS

	c, err := dialWithCA(t, addr, cfg, pool)
	if err != nil {
		t.Fatalf("connect() over STARTTLS failed: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Reached only on an upgraded connection: the server refuses to
	// authenticate before the upgrade, and go-imap refuses to try.
	if err := c.Login(testUser, testPass).Wait(); err != nil {
		t.Fatalf("login over the upgraded connection failed: %v", err)
	}
}

// The upgrade is mandatory, never opportunistic. A network attacker able to
// strip the STARTTLS advertisement from the greeting would otherwise be handed
// the password, which is the whole reason opportunistic encryption is not
// encryption.
func TestSTARTTLSIsRefusedWhenTheServerWillNotUpgrade(t *testing.T) {
	addr, _ := startSTARTTLSServer(t, nil)

	cfg := configFor(t, addr)
	cfg.Insecure = false
	cfg.Security = model.SecuritySTARTTLS

	be, err := Dial(context.Background(), cfg, testProvider(t, testPass))
	if err == nil {
		_ = be.Close()
		t.Fatal("the dialer carried on in the clear when the upgrade was unavailable; " +
			"the password would have gone over the wire")
	}
}

// The cleartext path exists for one reason: the in-memory server the engine is
// tested against speaks no TLS, and it is always on loopback. A comment saying
// "tests only" does not survive the first person who copies the struct
// literal.
func TestPlaintextIsRefusedForAnythingButLoopback(t *testing.T) {
	cases := []struct {
		host    string
		allowed bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"localhost", true},
		{"LOCALHOST", true},
		{"mail.example.com", false},
		{"10.0.0.5", false},
		{"192.168.1.10", false},
	}

	for _, c := range cases {
		err := checkSecurity(Config{Host: c.host, Port: 143, Insecure: true})
		if c.allowed && err != nil {
			t.Errorf("plaintext to %q was refused: %v", c.host, err)
		}
		if !c.allowed && err == nil {
			t.Errorf("plaintext to %q was allowed; a password would go over the wire", c.host)
		}
	}
}

// A private network address is still a network. The test server is on
// loopback; an internal mail server is not, and reaching it in the clear puts
// the password in front of everyone else on that network.
func TestPlaintextRefusalNamesWhatIsWrong(t *testing.T) {
	err := checkSecurity(Config{Host: "mail.sirket.local", Insecure: true})
	if err == nil {
		t.Fatal("no error for a cleartext connection to a real host")
	}
	if !strings.Contains(err.Error(), "mail.sirket.local") {
		t.Errorf("error %q does not say which host was refused", err)
	}
}

// Encryption is on unless a caller explicitly asks for the loopback-only
// cleartext path, and the zero value has to be the safe one: a Config built
// without thinking about security must not produce a cleartext connection.
func TestTheZeroConfigIsEncrypted(t *testing.T) {
	if err := checkSecurity(Config{Host: "mail.example.com", Port: 993}); err != nil {
		t.Errorf("a zero-value Config was refused: %v", err)
	}
	if model.SecurityOrDefault(string(Config{}.Security)) != model.SecurityTLS {
		t.Error("the zero-value Config does not mean implicit TLS")
	}
}

// Getting the port wrong is the commonest way this fails, and the generic
// handshake error says nothing about ports.
func TestTheDialErrorExplainsAPortMismatch(t *testing.T) {
	base := errors.New("read tcp 10.0.0.5:993: connection reset by peer")

	tls143 := describeDialError(Config{Security: model.SecurityTLS, Port: 143}, base)
	if !strings.Contains(tls143.Error(), "993") {
		t.Errorf("implicit TLS on 143 says %q, want it to name 993", tls143)
	}

	start993 := describeDialError(Config{Security: model.SecuritySTARTTLS, Port: 993}, base)
	if !strings.Contains(start993.Error(), "143") {
		t.Errorf("STARTTLS on 993 says %q, want it to name 143", start993)
	}

	// A correct pairing must not get a made-up explanation bolted onto a real
	// error — "the server is down" would then read as "your port is wrong".
	fine := describeDialError(Config{Security: model.SecurityTLS, Port: 993}, base)
	if fine.Error() != base.Error() {
		t.Errorf("a correct configuration was given the explanation %q", fine)
	}
}

// An internal certificate authority is the normal case for an on-premises
// server, and the generic Go message sends people looking in the wrong place.
func TestTheDialErrorExplainsAnUntrustedAuthority(t *testing.T) {
	err := describeDialError(Config{Security: model.SecurityTLS, Port: 993},
		x509.UnknownAuthorityError{})

	if !strings.Contains(err.Error(), "root certificate") {
		t.Errorf("error %q does not point at the missing root certificate", err)
	}
}
