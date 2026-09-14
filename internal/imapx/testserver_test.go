package imapx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"nexusmail/internal/auth"
)

const (
	testUser = "user@example.com"
	testPass = "password"
)

// serverCaps is what the fake server advertises.
//
// imapserver.Options documents that a nil Caps advertises IMAP4rev1 only, so
// MOVE and the rest have to be listed explicitly. Being explicit is also what
// lets a test start a server *without* CONDSTORE, which is the case the delta
// sync fallback exists for.
func serverCaps(extra ...imap.Cap) imap.CapSet {
	caps := imap.CapSet{
		imap.CapIMAP4rev1: {},
		imap.CapIMAP4rev2: {},
	}
	for _, c := range extra {
		caps[c] = struct{}{}
	}
	return caps
}

// startFakeServer runs a real in-memory IMAP server on loopback. This is not a
// mock: it speaks the protocol, so a wrong password really fails auth and
// UIDVALIDITY really comes from a SELECT response.
func startFakeServer(t *testing.T, caps imap.CapSet) (addr string, user *imapmemserver.User) {
	t.Helper()

	mem := imapmemserver.New()
	user = imapmemserver.NewUser(testUser, testPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(_ *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps: caps,
		// The fake server speaks plaintext on loopback. Production always uses
		// TLS; this is the only place a cleartext PLAIN exchange is allowed.
		InsecureAuth: true,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String(), user
}

func configFor(t *testing.T, addr string) Config {
	t.Helper()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return Config{Host: host, Port: port, Insecure: true, Username: testUser}
}

func testProvider(t *testing.T, password string) auth.CredentialProvider {
	t.Helper()

	store, err := auth.NewFileStore(t.TempDir(), "test-master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := store.Set("ref", password); err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	return auth.NewPasswordProvider(testUser, "ref", store)
}

// literal adapts a string to the imap.LiteralReader the memory server's
// Append expects.
type literal struct {
	*strings.Reader
	size int64
}

func newLiteral(s string) *literal {
	return &literal{Reader: strings.NewReader(s), size: int64(len(s))}
}

func (l *literal) Size() int64 { return l.size }

// appendMessage puts a message into a mailbox on the server side.
func appendMessage(t *testing.T, user *imapmemserver.User, mailbox, raw string) {
	t.Helper()
	if _, err := user.Append(mailbox, newLiteral(raw), &imap.AppendOptions{}); err != nil {
		t.Fatalf("append to %s: %v", mailbox, err)
	}
}

// htmlMessage builds a well-formed HTML mail with the headers the sync engine
// reads.
func htmlMessage(subject, from, body string) string {
	return "From: " + from + "\r\n" +
		"To: " + testUser + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Message-ID: <" + strings.ReplaceAll(subject, " ", "-") + "@example.com>\r\n" +
		"Date: Mon, 02 Jan 2026 15:04:05 +0000\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" + body + "\r\n"
}

// startSTARTTLSServer runs a plaintext listener that offers the upgrade, which
// is how on-premises Exchange publishes IMAP: port 143, encrypted only once
// the client asks for it.
//
// A nil tlsCfg makes the server refuse to upgrade, which is the case the
// mandatory-upgrade test needs.
func startSTARTTLSServer(t *testing.T, tlsCfg *tls.Config) (addr string, user *imapmemserver.User) {
	t.Helper()

	mem := imapmemserver.New()
	user = imapmemserver.NewUser(testUser, testPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(_ *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         serverCaps(),
		TLSConfig:    tlsCfg,
		InsecureAuth: true,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String(), user
}

// ownCA issues a certificate for 127.0.0.1 and returns a pool that trusts it.
//
// This is what an on-premises server looks like on a domain-joined machine:
// the certificate is issued by the organisation's own authority, which the
// machine trusts. Switching verification off to make the test pass would have
// tested nothing at all — the pool is what keeps the handshake real.
func ownCA(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}, pool
}
