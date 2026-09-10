package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"nexusmail/internal/auth"
	"nexusmail/internal/imapx"
	"nexusmail/internal/store"
	imapsync "nexusmail/internal/sync"
)

// This file drives the whole application against a real IMAP server over a
// real socket: real LOGIN, LIST, SELECT and FETCH, real MIME decoding, the
// real database, the real sanitiser and the real HTTP handler that serves
// bodies to the window.
//
// Every other test in this package uses stubBackend, which returns Go structs
// and never encodes anything. That catches logic errors and cannot catch a
// wrong FETCH item, a charset that is declared but never decoded, or a header
// the parser silently drops — the failures that only appear against something
// that actually speaks the protocol.

const (
	fnUser = "user@example.com"
	fnPass = "correct-horse"
)

// startIMAPServer runs a real in-memory IMAP server on loopback. Passing a TLS
// config wraps the listener, which is how the TLS assertions below reach a
// server that is genuinely speaking TLS rather than refusing the connection.
func startIMAPServer(t *testing.T, tlsCfg *tls.Config) (host string, port int, user *imapmemserver.User) {
	t.Helper()

	mem := imapmemserver.New()
	user = imapmemserver.NewUser(fnUser, fnPass)
	for _, mailbox := range []string{"INBOX", "Arşiv"} {
		if err := user.Create(mailbox, nil); err != nil {
			t.Fatalf("create %s: %v", mailbox, err)
		}
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps: imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		// Cleartext PLAIN is allowed only because this listener is on
		// loopback inside a test. DialerFor never sets TLS false.
		InsecureAuth: true,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, user
}

type rawLiteral struct {
	*strings.Reader
	size int64
}

func (l *rawLiteral) Size() int64 { return l.size }

func deliver(t *testing.T, user *imapmemserver.User, mailbox, raw string) {
	t.Helper()
	lit := &rawLiteral{Reader: strings.NewReader(raw), size: int64(len(raw))}
	if _, err := user.Append(mailbox, lit, &imap.AppendOptions{}); err != nil {
		t.Fatalf("append to %s: %v", mailbox, err)
	}
}

// newLiveService wires the real service against the running server. The engine
// gets a dialer built the same way DialerFor builds one — same provider, same
// imapx.Dial — with TLS off, because a self-signed certificate cannot be
// trusted by the system roots this platform checks against. What TLS itself
// does is asserted separately below.
func newLiveService(t *testing.T, host string, port int) (*MailService, *store.Store) {
	t.Helper()

	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open() error: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})

	secrets, err := auth.NewFileStore(dir, "test-master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}

	eng := imapsync.New(db, func(ctx context.Context, accountID int64) (imapx.MailBackend, error) {
		acct, err := db.GetAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		provider := auth.NewPasswordProvider(acct.Email, acct.SecretRef, secrets)
		return imapx.Dial(ctx, imapx.Config{
			Host: acct.IMAPHost, Port: acct.IMAPPort, TLS: false, Username: acct.Email,
		}, provider)
	})

	return NewMailService(db, secrets, eng, Config{Emit: func(string, any) {}}), db
}

// TestReadingMailEndToEndAgainstARealServer is the acceptance test for M1: a
// person adds an account, their folders and messages appear, they can search
// them, and a message renders without leaking anything to its sender.
func TestReadingMailEndToEndAgainstARealServer(t *testing.T) {
	host, port, user := startIMAPServer(t, nil)

	// A tracker the message will try to reach. Nothing may ever hit it.
	var trackerHits atomic.Int32
	tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		trackerHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer tracker.Close()

	// Three messages that between them cover what breaks in real mail: a
	// non-ASCII subject, a charset that is declared rather than UTF-8, and a
	// body that tries to phone home.
	deliver(t, user, "INBOX", "From: Muhasebe <muhasebe@example.com>\r\n"+
		"To: "+fnUser+"\r\n"+
		"Subject: =?UTF-8?B?xZ51YmF0IG11dGFiYWthdMSx?=\r\n"+
		"Message-ID: <subat@example.com>\r\n"+
		"Date: Mon, 02 Feb 2026 09:30:00 +0300\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n\r\n"+
		`<p>Şubat ayı mutabakatı ektedir.</p>`+
		`<img src="`+tracker.URL+`/pixel.gif" width="1" height="1">`+
		`<script>fetch("`+tracker.URL+`/beacon")</script>`+"\r\n")

	// ISO-8859-9 is what Turkish mail servers still emit. Declared in the
	// header, so the parser has to honour it rather than assume UTF-8.
	latin5Subject := "=?ISO-8859-9?Q?Toplant=FD_notlar=FD?=" // Toplantı notları
	latin5Body := "<p>" + string([]byte{0x53, 0x61, 0x6c, 0xfd}) + " g" +
		string([]byte{0xfc}) + "n" + string([]byte{0xfc}) + "</p>" // Salı günü
	deliver(t, user, "INBOX", "From: Zeynep <zeynep@example.com>\r\n"+
		"To: "+fnUser+"\r\n"+
		"Subject: "+latin5Subject+"\r\n"+
		"Message-ID: <toplanti@example.com>\r\n"+
		"Date: Tue, 03 Feb 2026 11:00:00 +0300\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=iso-8859-9\r\n\r\n"+
		latin5Body+"\r\n")

	deliver(t, user, "Arşiv", "From: Bordro <bordro@example.com>\r\n"+
		"To: "+fnUser+"\r\n"+
		"Subject: Gecen yilin bordrosu\r\n"+
		"Message-ID: <bordro@example.com>\r\n"+
		"Date: Wed, 04 Feb 2026 08:00:00 +0300\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n\r\n"+
		"Arsivden.\r\n")

	svc, db := newLiveService(t, host, port)

	// --- adding the account -------------------------------------------------
	acct, err := svc.AddPasswordAccount(fnUser, "Test", host, port, "", 0, fnPass)
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	// --- folders ------------------------------------------------------------
	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	byName := map[string]FolderDTO{}
	for _, f := range folders {
		byName[f.Name] = f
	}
	for _, want := range []string{"INBOX", "Arşiv"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("folder %q missing; got %v", want, folders)
		}
	}
	if !byName["INBOX"].IsInbox {
		t.Error("INBOX was not recognised as the inbox")
	}

	// --- the inbox ----------------------------------------------------------
	inbox, err := svc.OpenFolder(byName["INBOX"].ID, 50)
	if err != nil {
		t.Fatalf("OpenFolder(INBOX) error: %v", err)
	}
	if len(inbox) != 2 {
		t.Fatalf("INBOX holds %d messages, want 2", len(inbox))
	}

	subjects := map[string]MessageDTO{}
	for _, m := range inbox {
		subjects[m.Subject] = m
	}
	// A subject that survives the round trip proves RFC 2047 decoding, which a
	// stub backend can never exercise because it never encodes anything.
	if _, ok := subjects["Şubat mutabakatı"]; !ok {
		t.Errorf("the UTF-8 subject did not decode; got %v", keysOf(subjects))
	}
	if _, ok := subjects["Toplantı notları"]; !ok {
		t.Errorf("the ISO-8859-9 subject did not decode; got %v", keysOf(subjects))
	}

	if got := subjects["Şubat mutabakatı"].FromName; got != "Muhasebe" {
		t.Errorf("sender name = %q, want %q", got, "Muhasebe")
	}

	// The list is ordered by the server's INTERNALDATE, not by the Date
	// header, and these messages were delivered just now. A sender can put
	// anything in Date:, including a time far in the future that pins their
	// mail to the top of the list forever, which is why the header is not what
	// the list sorts on.
	if age := time.Since(time.Unix(subjects["Şubat mutabakatı"].InternalDateUnix, 0)); age > time.Minute {
		t.Errorf("internal date is %s old; it should be the delivery time", age)
	}

	// The header is still parsed and stored. RFC 2822 dates carry an offset
	// (+0300 here) and a parser that dropped the zone would land three hours
	// out — invisible in a list sorted on something else, and wrong the moment
	// the reading pane shows it.
	stored, err := db.ListMessages(context.Background(), byName["INBOX"].ID, 50, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	wantHeaderDate := time.Date(2026, 2, 2, 9, 30, 0, 0, time.FixedZone("", 3*3600))
	var checked bool
	for _, m := range stored {
		if m.Subject != "Şubat mutabakatı" {
			continue
		}
		checked = true
		if !m.Date.Equal(wantHeaderDate) {
			t.Errorf("Date header parsed as %s, want %s", m.Date, wantHeaderDate)
		}
	}
	if !checked {
		t.Error("the message was not found in the store")
	}

	// --- a folder the initial sync deliberately left alone ------------------
	archive, err := svc.OpenFolder(byName["Arşiv"].ID, 50)
	if err != nil {
		t.Fatalf("OpenFolder(Arşiv) error: %v", err)
	}
	if len(archive) != 1 {
		t.Fatalf("Arşiv holds %d messages, want 1", len(archive))
	}

	// --- search -------------------------------------------------------------
	t.Run("search reaches across folders and tolerates a missing dot", func(t *testing.T) {
		for _, tc := range []struct{ query, want string }{
			{"şubat", "Şubat mutabakatı"},
			{"subat", "Şubat mutabakatı"},      // no Turkish keyboard
			{"mutabakati", "Şubat mutabakatı"}, // dotless ı typed as i
			{"toplanti", "Toplantı notları"},   // same, on the Latin-5 message
			{"muhasebe", "Şubat mutabakatı"},   // sender address
			{"bordro", "Gecen yilin bordrosu"}, // a different folder entirely
		} {
			got, err := svc.SearchMessages(acct.ID, tc.query, 50)
			if err != nil {
				t.Fatalf("SearchMessages(%q) error: %v", tc.query, err)
			}
			if len(got) != 1 {
				t.Errorf("SearchMessages(%q) returned %d results, want 1", tc.query, len(got))
				continue
			}
			if got[0].Subject != tc.want {
				t.Errorf("SearchMessages(%q) found %q, want %q", tc.query, got[0].Subject, tc.want)
			}
		}
	})

	// --- reading a body through the real HTTP handler ------------------------
	handler := NewBodyHandler(svc)
	messageID := subjects["Şubat mutabakatı"].ID

	t.Run("the body renders with nothing reaching the sender", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mail-body/"+
			strconv.FormatInt(messageID, 10), nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()

		// The message decoded and reached the page.
		if !strings.Contains(body, "Şubat ayı mutabakatı ektedir.") {
			t.Errorf("the message text is missing from the rendered body")
		}
		// The script is gone with its contents. bluemonday's default is to
		// drop a disallowed element but keep its text, which would have put
		// the beacon URL on screen as visible source code.
		if strings.Contains(strings.ToLower(body), "<script") ||
			strings.Contains(body, "/beacon") {
			t.Errorf("the script survived sanitisation:\n%s", body)
		}

		// The tracker's address does survive, deliberately, parked on a
		// data- attribute so consent can restore it. What must not survive is
		// any attribute a browser would act on.
		// The space matters: without it, "src=" also matches the tail of
		// data-nexus-blocked-src and the check fails on the very marker it is
		// supposed to allow.
		for _, attr := range []string{" src=", " srcset=", " href=", " background=", " style="} {
			if strings.Contains(body, attr+`"`+tracker.URL) {
				t.Errorf("the tracker is reachable through %s:\n%s", attr, body)
			}
		}
		if !strings.Contains(body, "data-nexus-blocked-src") {
			t.Errorf("the blocked image left no marker, so consent has nothing to restore")
		}

		// Belt and braces: the policy header forbids the network outright, so
		// a mistake in the rewriting above still cannot become a request.
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("Content-Security-Policy = %q, want default-src 'none'", csp)
		}
		if strings.Contains(csp, tracker.URL) || strings.Contains(csp, "img-src *") {
			t.Errorf("the policy permits the network: %q", csp)
		}

		if got := trackerHits.Load(); got != 0 {
			t.Errorf("the tracker was contacted %d times while rendering", got)
		}
	})

	t.Run("loading images routes them through the app, not the reader", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mail-body/"+
			strconv.FormatInt(messageID, 10)+"?remote=1", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()

		if strings.Contains(body, tracker.URL) {
			t.Fatalf("consent rewrote the image to the sender's own URL, so the "+
				"browser would connect directly:\n%s", body)
		}

		srcs := regexp.MustCompile(`src="([^"]+)"`).FindStringSubmatch(body)
		if len(srcs) < 2 {
			t.Fatalf("no image survived the proxy render:\n%s", body)
		}
		if !strings.Contains(srcs[1], assetPath) {
			t.Errorf("image src = %q, want it pointing at the app's proxy", srcs[1])
		}

		// Following the proxy link must not reach a loopback address. The
		// image proxy is a request this application makes on the reader's
		// behalf, and without this guard a message could use it to probe the
		// machine and the network it sits on.
		assetRec := httptest.NewRecorder()
		handler.ServeHTTP(assetRec, httptest.NewRequest(http.MethodGet, srcs[1], nil))

		if assetRec.Code == http.StatusOK {
			t.Errorf("the proxy fetched a loopback address; status = %d", assetRec.Code)
		}
		if got := trackerHits.Load(); got != 0 {
			t.Errorf("the tracker was contacted %d times in total", got)
		}
	})
}

func keysOf(m map[string]MessageDTO) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// selfSignedFor issues a certificate for 127.0.0.1. It is deliberately not
// trusted by anything: the tests below are about what happens when a
// certificate cannot be verified.
func selfSignedFor(t *testing.T) *tls.Config {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}
}

// The dialer the application actually uses is the one place a password could
// end up on the wire in the clear, and the one place certificate checking
// could be switched off. Neither is visible by reading a happy-path test, so
// both are asserted directly.
func TestTheRealDialerRefusesAnythingButVerifiedTLS(t *testing.T) {
	newDialer := func(t *testing.T, host string, port int) imapsync.Dialer {
		t.Helper()
		dir := t.TempDir()
		db, err := store.Open(dir)
		if err != nil {
			t.Fatalf("store.Open() error: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		secrets, err := auth.NewFileStore(dir, "test-master")
		if err != nil {
			t.Fatalf("NewFileStore() error: %v", err)
		}
		svc := NewMailService(db, secrets, nil, Config{Emit: func(string, any) {}})
		if _, err := svc.AddPasswordAccount(fnUser, "T", host, port, "", 0, fnPass); err != nil {
			t.Fatalf("AddPasswordAccount() error: %v", err)
		}
		return DialerFor(db, secrets, Config{})
	}

	t.Run("a plaintext server is not silently accepted", func(t *testing.T) {
		host, port, _ := startIMAPServer(t, nil)
		dial := newDialer(t, host, port)

		be, err := dial(context.Background(), 1)
		if err == nil {
			_ = be.Close()
			t.Fatal("the dialer connected to a cleartext server; the password " +
				"would have gone over the wire in the clear")
		}
	})

	t.Run("an unverifiable certificate is refused", func(t *testing.T) {
		host, port, _ := startIMAPServer(t, selfSignedFor(t))
		dial := newDialer(t, host, port)

		be, err := dial(context.Background(), 1)
		if err == nil {
			_ = be.Close()
			t.Fatal("the dialer accepted a self-signed certificate, so " +
				"verification is off somewhere")
		}
		// Distinguishes "verification rejected it" from "nothing was
		// listening", which would make this test pass for the wrong reason.
		var certErr *tls.CertificateVerificationError
		if !errors.As(err, &certErr) {
			t.Errorf("error was %v, want a certificate verification failure", err)
		}
	})
}
