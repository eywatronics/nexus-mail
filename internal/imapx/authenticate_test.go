package imapx

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
)

// restrictedSession advertises only the mechanisms a test names, which is how
// a server whose administrator has locked basic authentication down looks from
// the outside. On-premises Exchange is the reason this matters: its IMAP4
// service offers different mechanisms depending on its LoginType.
type restrictedSession struct {
	imapserver.Session
	mechs []string
	// refuseLogin makes the LOGIN command fail, which is what a server whose
	// administrator has disabled basic authentication does. Advertising NTLM
	// alone is not enough to reproduce it: without LOGINDISABLED the command
	// is worth trying, and a fake that accepted it would test the opposite of
	// what it claims to.
	refuseLogin bool
}

func (s *restrictedSession) AuthenticateMechanisms() []string { return s.mechs }

func (s *restrictedSession) Authenticate(mech string) (sasl.Server, error) {
	switch mech {
	case "PLAIN":
		return sasl.NewPlainServer(func(_, username, password string) error {
			return s.Login(username, password)
		}), nil
	case "LOGIN":
		return &loginServer{login: s.Login}, nil
	default:
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "unsupported mechanism"}
	}
}

func (s *restrictedSession) Login(username, password string) error {
	if s.refuseLogin {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "basic authentication is disabled on this server",
		}
	}
	return s.Session.Login(username, password)
}

// loginServer is the server half of SASL LOGIN. go-sasl ships only the client,
// and a real exchange is worth more here than a stub that always says yes.
type loginServer struct {
	login    func(username, password string) error
	username string
	asked    bool
}

func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	if !s.asked {
		s.username = string(response)
		s.asked = true
		return []byte("Password:"), false, nil
	}
	if err := s.login(s.username, string(bytes.TrimSpace(response))); err != nil {
		return nil, false, err
	}
	return nil, true, nil
}

// startServerWithMechanisms runs a loopback server advertising exactly the
// mechanisms given. An empty list means it advertises none at all, which is
// what an old server offering only the LOGIN command looks like.
func startServerWithMechanisms(t *testing.T, mechs []string, insecureAuth bool) string {
	return startServerRefusingLogin(t, mechs, insecureAuth, false)
}

func startServerRefusingLogin(t *testing.T, mechs []string, insecureAuth, refuseLogin bool) string {
	t.Helper()

	mem := imapmemserver.New()
	user := imapmemserver.NewUser(testUser, testPass)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(_ *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &restrictedSession{
				Session: mem.NewSession(), mechs: mechs, refuseLogin: refuseLogin,
			}, nil, nil
		},
		// IMAP4rev1 only. The wrapper below hides the optional session
		// interfaces the memory server implements, and imapserver panics when
		// it advertises a capability the session cannot serve.
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: insecureAuth,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

// A server that does not offer PLAIN used to answer with a bare NO, and the
// reader was told "authentication failed" about a password that was perfectly
// correct.
func TestAuthenticationFallsBackToSASLLogin(t *testing.T) {
	addr := startServerWithMechanisms(t, []string{"LOGIN"}, true)

	be, err := Dial(context.Background(), configFor(t, addr), testProvider(t, testPass))
	if err != nil {
		t.Fatalf("Dial() against a LOGIN-only server failed: %v", err)
	}
	defer func() { _ = be.Close() }()

	if _, err := be.Select(context.Background(), "INBOX"); err != nil {
		t.Errorf("Select() after SASL LOGIN failed: %v", err)
	}
}

// A server advertising no SASL mechanism at all has not said LOGIN is
// disabled, so the command is worth trying — older servers offer nothing else.
func TestAuthenticationFallsBackToTheLoginCommand(t *testing.T) {
	addr := startServerWithMechanisms(t, []string{}, true)

	be, err := Dial(context.Background(), configFor(t, addr), testProvider(t, testPass))
	if err != nil {
		t.Fatalf("Dial() against a server with no SASL mechanisms failed: %v", err)
	}
	defer func() { _ = be.Close() }()

	if _, err := be.Select(context.Background(), "INBOX"); err != nil {
		t.Errorf("Select() after LOGIN failed: %v", err)
	}
}

// PLAIN stays the first choice where it is offered: it is the mechanism every
// modern server implements and the one with no obsolete challenge exchange.
func TestAuthenticationPrefersPlain(t *testing.T) {
	addr := startServerWithMechanisms(t, []string{"PLAIN", "LOGIN"}, true)

	be, err := Dial(context.Background(), configFor(t, addr), testProvider(t, testPass))
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	_ = be.Close()
}

// A wrong password must still fail. A fallback chain that ended in something
// permissive would be worse than no fallback at all.
func TestTheFallbackChainStillRejectsAWrongPassword(t *testing.T) {
	for _, mechs := range [][]string{{"LOGIN"}, {}} {
		addr := startServerWithMechanisms(t, mechs, true)

		be, err := Dial(context.Background(), configFor(t, addr), testProvider(t, "yanlis"))
		if err == nil {
			_ = be.Close()
			t.Errorf("a wrong password was accepted with mechanisms %v", mechs)
		}
	}
}

// When a site has locked Exchange down to NTLM and GSSAPI, every password the
// reader types is rejected and none of them are wrong. Being told what the
// server asked for is what turns retyping into a conversation with whoever
// configured it.
func TestAFailedFallbackSaysWhatTheServerOffers(t *testing.T) {
	addr := startServerRefusingLogin(t, []string{"NTLM", "GSSAPI"}, true, true)

	_, err := Dial(context.Background(), configFor(t, addr), testProvider(t, testPass))
	if err == nil {
		t.Fatal("Dial() succeeded against a server offering only NTLM and GSSAPI")
	}
	for _, want := range []string{"NTLM", "GSSAPI"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// LOGINDISABLED is the server saying the door is shut, not that it is stiff.
// Trying anyway would produce a worse error than saying so.
func TestAServerThatRefusesPasswordsIsReportedAsSuch(t *testing.T) {
	addr := startServerWithMechanisms(t, []string{}, false)

	_, err := Dial(context.Background(), configFor(t, addr), testProvider(t, testPass))
	if err == nil {
		t.Fatal("Dial() succeeded against a server advertising LOGINDISABLED")
	}
	if !strings.Contains(err.Error(), "will not take a password") {
		t.Errorf("error %q does not say the server refuses passwords", err)
	}
}

func TestTheMechanismListIsReadable(t *testing.T) {
	caps := imap.CapSet{
		imap.CapIMAP4rev1:      {},
		imap.AuthCap("GSSAPI"): {},
		imap.AuthCap("NTLM"):   {},
	}
	if got := describeMechanisms(caps); got != "GSSAPI, NTLM" {
		t.Errorf("describeMechanisms() = %q, want a sorted list", got)
	}
	if got := describeMechanisms(imap.CapSet{imap.CapIMAP4rev1: {}}); got != "none" {
		t.Errorf("describeMechanisms() with no mechanisms = %q, want none", got)
	}
}
