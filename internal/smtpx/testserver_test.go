package smtpx

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// A real SMTP server, not a mock.
//
// The project rule is that every protocol comes with its own fake server, and
// the reason is the same one that makes the IMAP tests worth having: a mock
// agrees with whatever the client does, so it confirms the client talks to
// itself. go-smtp's server side speaks the protocol, which means a client that
// gets MAIL FROM wrong fails here rather than against somebody's mailbox.

// recorded is what the server received, so a test can assert on the wire
// rather than on the client's own idea of what it sent.
type recorded struct {
	mu   sync.Mutex
	from string
	to   []string
	body []byte
	// utf8 and bodyType are the MAIL parameters, which is where 8BITMIME and
	// SMTPUTF8 actually show up.
	utf8     bool
	bodyType string
	size     int64
	// mechanism is how the client signed in, for the tests that care which.
	mechanism string
}

func (r *recorded) snapshot() recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return recorded{from: r.from, to: append([]string(nil), r.to...), body: r.body,
		utf8: r.utf8, bodyType: r.bodyType, size: r.size, mechanism: r.mechanism}
}

// serverOptions is what a test wants to vary about the server it talks to.
type serverOptions struct {
	// password empty means the server accepts any credentials.
	password string
	// authMechs overrides what the server advertises. Nil means the default.
	authMechs []string
	// username and token are what an XOAUTH2 exchange must carry.
	username string
	token    string
	// maxSize is advertised as SIZE. Zero advertises none.
	maxSize int64
	// rejectRecipient, when non-empty, is refused at RCPT.
	rejectRecipient string
	// failAtData refuses the message at the end of DATA, which is where a
	// content filter or a quota rejects it.
	failAtData bool
}

type testBackend struct {
	opts serverOptions
	got  *recorded
}

func (b *testBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &testSession{backend: b}, nil
}

type testSession struct {
	backend       *testBackend
	authenticated bool
}

func (s *testSession) AuthMechanisms() []string {
	if s.backend.opts.authMechs != nil {
		return s.backend.opts.authMechs
	}
	return []string{sasl.Plain}
}

func (s *testSession) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			if s.backend.opts.password != "" && password != s.backend.opts.password {
				return smtp.ErrAuthFailed
			}
			s.authenticated = true
			return nil
		}), nil
	case "XOAUTH2":
		// go-sasl ships no XOAUTH2 server, so the wire format is checked here:
		// user=<name>\x01auth=Bearer <token>\x01\x01. Checked rather than
		// waved through, because a client that sends the wrong shape would
		// otherwise pass this test and fail against Gmail.
		return xoauth2Server{session: s}, nil
	default:
		return nil, smtp.ErrAuthUnknownMechanism
	}
}

// xoauth2Server accepts the one exchange XOAUTH2 has: everything is in the
// initial response, and a correct token means there is nothing to say back.
type xoauth2Server struct {
	session *testSession
}

func (x xoauth2Server) Next(response []byte) (challenge []byte, done bool, err error) {
	want := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01",
		x.session.backend.opts.username, x.session.backend.opts.token)
	if string(response) != want {
		return nil, false, smtp.ErrAuthFailed
	}
	x.session.authenticated = true
	x.session.backend.got.mu.Lock()
	x.session.backend.got.mechanism = "XOAUTH2"
	x.session.backend.got.mu.Unlock()
	return nil, true, nil
}

func (s *testSession) Mail(from string, opts *smtp.MailOptions) error {
	s.backend.got.mu.Lock()
	defer s.backend.got.mu.Unlock()
	s.backend.got.from = from
	if opts != nil {
		s.backend.got.utf8 = opts.UTF8
		s.backend.got.bodyType = string(opts.Body)
		s.backend.got.size = opts.Size
	}
	return nil
}

func (s *testSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.backend.opts.rejectRecipient != "" && to == s.backend.opts.rejectRecipient {
		return &smtp.SMTPError{Code: 550, Message: "no such user here"}
	}
	s.backend.got.mu.Lock()
	defer s.backend.got.mu.Unlock()
	s.backend.got.to = append(s.backend.got.to, to)
	return nil
}

func (s *testSession) Data(r io.Reader) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if s.backend.opts.failAtData {
		return &smtp.SMTPError{Code: 552, Message: "message rejected by policy"}
	}
	s.backend.got.mu.Lock()
	defer s.backend.got.mu.Unlock()
	s.backend.got.body = body
	return nil
}

func (s *testSession) Reset()        {}
func (s *testSession) Logout() error { return nil }

// startServer runs a loopback SMTP server and returns its config and what it
// received.
func startServer(t *testing.T, opts serverOptions) (Config, *recorded) {
	t.Helper()

	got := &recorded{}
	be := &testBackend{opts: opts, got: got}

	srv := smtp.NewServer(be)
	srv.Domain = "localhost"
	srv.AllowInsecureAuth = true
	srv.EnableSMTPUTF8 = true
	srv.MaxMessageBytes = opts.maxSize

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	host, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("splitting the listener address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parsing the port: %v", err)
	}

	return Config{Host: host, Port: port, Insecure: true, Username: "u@example.com"}, got
}

// message builds a minimal RFC 5322 message.
func message(subject, body string) []byte {
	return []byte(strings.Join([]string{
		"From: u@example.com",
		"To: r@example.com",
		"Subject: " + subject,
		"",
		body,
		"",
	}, "\r\n"))
}
