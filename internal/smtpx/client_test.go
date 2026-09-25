package smtpx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"nexusmail/internal/auth"
	"nexusmail/internal/model"
)

// passwordCredential is what a generic server gets: a secret it may ask for
// as PLAIN or as LOGIN.
type passwordCredential struct {
	username, secret string
	reads            int
}

func (p *passwordCredential) SASLClient(context.Context) (sasl.Client, error) {
	return sasl.NewPlainClient("", p.username, p.secret), nil
}
func (p *passwordCredential) Refresh(context.Context) error { return nil }
func (p *passwordCredential) Kind() model.AuthKind          { return model.AuthPassword }
func (p *passwordCredential) Username() string              { return p.username }
func (p *passwordCredential) Password(context.Context) (string, error) {
	p.reads++
	return p.secret, nil
}

// tokenCredential is an OAuth account: no password to offer, only a minted
// token presented as XOAUTH2. It deliberately does not implement
// PasswordCredential, which is what sends smtpx down the other path.
type tokenCredential struct {
	username, token string
	err             error
}

func (c tokenCredential) SASLClient(context.Context) (sasl.Client, error) {
	if c.err != nil {
		return nil, c.err
	}
	return auth.NewXOAUTH2Client(c.username, c.token), nil
}
func (c tokenCredential) Refresh(context.Context) error { return nil }
func (c tokenCredential) Kind() model.AuthKind          { return model.AuthOAuth }

func dial(t *testing.T, cfg Config, secret string) *Client {
	t.Helper()
	return dialWith(t, cfg, &passwordCredential{username: "u@example.com", secret: secret})
}

func dialWith(t *testing.T, cfg Config, provider auth.CredentialProvider) *Client {
	t.Helper()

	c, err := Dial(context.Background(), cfg, provider)
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// The whole point of the package, against a server that speaks the protocol.
func TestAMessageReachesTheServerIntact(t *testing.T) {
	cfg, got := startServer(t, serverOptions{})
	c := dial(t, cfg, "pw")

	env := Envelope{From: "u@example.com", To: []string{"r@example.com"}}
	if err := c.Send(context.Background(), env, message("Merhaba", "gövde")); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	s := got.snapshot()
	if s.from != "u@example.com" {
		t.Errorf("MAIL FROM = %q", s.from)
	}
	if len(s.to) != 1 || s.to[0] != "r@example.com" {
		t.Errorf("RCPT TO = %v", s.to)
	}
	if !strings.Contains(string(s.body), "Subject: Merhaba") {
		t.Errorf("the message body did not arrive: %q", s.body)
	}
}

// The envelope is not the headers. A bcc recipient appears in one and in
// neither of the others, and a client that took the recipients from the
// headers would silently fail to deliver to them.
func TestTheEnvelopeIsSeparateFromTheHeaders(t *testing.T) {
	cfg, got := startServer(t, serverOptions{})
	c := dial(t, cfg, "pw")

	env := Envelope{
		From: "bounces@example.com",
		To:   []string{"r@example.com", "hidden@example.com"},
	}
	// The message names only one recipient; the envelope carries two.
	if err := c.Send(context.Background(), env, message("Konu", "metin")); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	s := got.snapshot()
	if s.from != "bounces@example.com" {
		t.Errorf("the return path was taken from the headers: %q", s.from)
	}
	if len(s.to) != 2 {
		t.Fatalf("RCPT TO = %v, want both recipients", s.to)
	}
	if strings.Contains(string(s.body), "hidden@example.com") {
		t.Error("the hidden recipient leaked into the message")
	}
}

func TestSendRefusesAnIncompleteEnvelope(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{})
	c := dial(t, cfg, "pw")
	ctx := context.Background()
	raw := message("Konu", "metin")

	if err := c.Send(ctx, Envelope{To: []string{"r@example.com"}}, raw); err == nil {
		t.Error("Send() accepted a message with no return path")
	}
	if err := c.Send(ctx, Envelope{From: "u@example.com"}, raw); err == nil {
		t.Error("Send() accepted a message with no recipients")
	}
	if err := c.Send(ctx, Envelope{From: "u@example.com", To: []string{"r@example.com"}}, nil); err == nil {
		t.Error("Send() accepted an empty message")
	}
}

// A refused recipient has to name the failure. The queue decides whether to
// retry from the error, and "something went wrong" is not something to decide
// on.
func TestARefusedRecipientIsReported(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{rejectRecipient: "nobody@example.com"})
	c := dial(t, cfg, "pw")

	env := Envelope{From: "u@example.com", To: []string{"nobody@example.com"}}
	err := c.Send(context.Background(), env, message("Konu", "metin"))
	if err == nil {
		t.Fatal("Send() reported success for a refused recipient")
	}
	if !strings.Contains(err.Error(), "recipient") {
		t.Errorf("the error does not say a recipient was refused: %v", err)
	}
}

// The close of the data stream is the send. A write that succeeds and a close
// that fails is a message the server did not accept, and reporting success
// would leave the user believing mail went out.
func TestAMessageRejectedAtTheEndOfDataIsNotASuccess(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{failAtData: true})
	c := dial(t, cfg, "pw")

	env := Envelope{From: "u@example.com", To: []string{"r@example.com"}}
	if err := c.Send(context.Background(), env, message("Konu", "metin")); err == nil {
		t.Fatal("Send() reported success for a message the server rejected")
	}
}

func TestWrongCredentialsAreReportedWithoutTheSecret(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{password: "correct-horse"})

	_, err := Dial(context.Background(), cfg,
		&passwordCredential{username: "u@example.com", secret: "CANARY-WRONG-PASSWORD"})
	if err == nil {
		t.Fatal("Dial() accepted the wrong password")
	}
	// The password must not travel in an error that ends up in a log or on
	// screen, which is the same rule the rest of the app follows.
	if strings.Contains(err.Error(), "CANARY-WRONG-PASSWORD") {
		t.Errorf("the password is in the error: %v", err)
	}
}

// Plaintext is for the loopback test server and nothing else. A comment saying
// so does not survive the first person who copies the struct literal.
func TestCleartextIsRefusedOffLoopback(t *testing.T) {
	for _, host := range []string{"smtp.example.com", "203.0.113.10"} {
		_, err := Dial(context.Background(),
			Config{Host: host, Port: 25, Insecure: true, Username: "u@example.com"},
			&passwordCredential{username: "u@example.com", secret: "pw"})
		if err == nil {
			t.Errorf("Dial() allowed cleartext to %s", host)
			continue
		}
		if !strings.Contains(err.Error(), "refusing an unencrypted connection") {
			t.Errorf("%s: unexpected error %v", host, err)
		}
	}
}

func TestCleartextIsAllowedOnLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "::1"} {
		if err := checkSecurity(Config{Host: host, Insecure: true}); err != nil {
			t.Errorf("checkSecurity refused loopback host %s: %v", host, err)
		}
	}
}

// Picking implicit TLS on the STARTTLS port fails deep inside the handshake,
// and the error a user sees is about a record header. It has to say which of
// the two they got wrong.
func TestTheTwoSubmissionPortMistakesAreNamed(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			"implicit TLS on 587",
			Config{Host: "127.0.0.1", Port: 587},
			"587 is the STARTTLS submission port",
		},
		{
			"STARTTLS on 465",
			Config{Host: "127.0.0.1", Port: 465, Security: model.SecuritySTARTTLS},
			"465 is the implicit-TLS submission port",
		},
	}

	for _, c := range cases {
		// Nothing is listening, so the dial fails and the wrapper runs. The
		// message is what is under test, not the connection.
		_, err := Dial(context.Background(), c.cfg,
			&passwordCredential{username: "u@example.com", secret: "pw"})
		if err == nil {
			t.Errorf("%s: Dial() succeeded against nothing", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not explain the mistake", c.name, err)
		}
	}
}

// A server that declares SIZE rejects an over-large message only after the
// whole body has crossed the wire. Checking first saves the upload, and saves
// the queue from spending it again on a retry.
func TestAnOverlargeMessageIsRefusedBeforeItIsUploaded(t *testing.T) {
	cfg, got := startServer(t, serverOptions{maxSize: 200})
	c := dial(t, cfg, "pw")

	if c.Capabilities().MaxMessageSize != 200 {
		t.Fatalf("SIZE was read as %d, want 200", c.Capabilities().MaxMessageSize)
	}

	big := message("Konu", strings.Repeat("x", 500))
	env := Envelope{From: "u@example.com", To: []string{"r@example.com"}}
	err := c.Send(context.Background(), env, big)
	if err == nil {
		t.Fatal("Send() uploaded a message the server had already said was too big")
	}
	if !strings.Contains(err.Error(), "accepts at most") {
		t.Errorf("the error does not name the limit: %v", err)
	}
	// Nothing reached the server: not even MAIL FROM.
	if s := got.snapshot(); s.from != "" {
		t.Errorf("the transaction started anyway: from=%q", s.from)
	}
}

// What the server offered decides what may be put on the wire, so it has to be
// read rather than assumed.
func TestCapabilitiesComeFromTheServer(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{})
	caps := dial(t, cfg, "pw").Capabilities()

	if !caps.SMTPUTF8 {
		t.Error("SMTPUTF8 was advertised and not recorded")
	}
	if len(caps.Auth) == 0 {
		t.Error("no authentication mechanisms were recorded")
	}
	// The test server declares no SIZE, and zero has to mean unknown rather
	// than unlimited — which is why nothing is refused on the strength of it.
	if caps.MaxMessageSize != 0 {
		t.Errorf("MaxMessageSize = %d with no SIZE advertised", caps.MaxMessageSize)
	}
}

// 8BITMIME is not decoration: a message with raw 8-bit bytes sent to a server
// without it is a protocol violation that servers handle inconsistently.
func TestEightBitContentIsDeclaredWhenTheServerOffersIt(t *testing.T) {
	cfg, got := startServer(t, serverOptions{})
	c := dial(t, cfg, "pw")

	if !c.Capabilities().EightBitMIME {
		t.Skip("this server does not offer 8BITMIME, so there is nothing to declare")
	}

	env := Envelope{From: "u@example.com", To: []string{"r@example.com"}}
	if err := c.Send(context.Background(), env, message("Konu", "ışık ve çağrı")); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if s := got.snapshot(); s.bodyType != "8BITMIME" {
		t.Errorf("BODY= was %q, want 8BITMIME", s.bodyType)
	}
}

// The refusal happens locally, before the transaction, because the alternative
// is a rejection the user reads as a server fault.
func TestEightBitContentIsRefusedWhenTheServerLacks8BITMIME(t *testing.T) {
	c := &Client{caps: Capabilities{}}

	env := Envelope{From: "u@example.com", To: []string{"r@example.com"}}
	err := c.checkAcceptable(env, message("Konu", "ışık"))
	if err == nil {
		t.Fatal("an 8-bit message was accepted for a 7-bit-only server")
	}
	if !strings.Contains(err.Error(), "8BITMIME") {
		t.Errorf("the error does not name the missing extension: %v", err)
	}
}

func TestANonASCIIAddressNeedsSMTPUTF8(t *testing.T) {
	c := &Client{caps: Capabilities{EightBitMIME: true}}
	env := Envelope{From: "çağrı@example.com", To: []string{"r@example.com"}}

	err := c.checkAcceptable(env, message("Konu", "metin"))
	if err == nil {
		t.Fatal("a non-ASCII address was accepted without SMTPUTF8")
	}
	if !strings.Contains(err.Error(), "SMTPUTF8") {
		t.Errorf("the error does not name the missing extension: %v", err)
	}

	withIt := &Client{caps: Capabilities{EightBitMIME: true, SMTPUTF8: true}}
	if err := withIt.checkAcceptable(env, message("Konu", "metin")); err != nil {
		t.Errorf("a non-ASCII address was refused by a server that offers SMTPUTF8: %v", err)
	}
}

// The hostname is frequently a person's name and their employer, and it
// travels in the Received headers of every message they send.
func TestTheClientDoesNotAnnounceTheMachinesName(t *testing.T) {
	if ehloName != "localhost" {
		t.Errorf("EHLO says %q; it must not identify the machine", ehloName)
	}
}

// The queue decides whether to retry from this, and both mistakes are bad in
// their own way: a message the server will never accept retried forever never
// leaves and never says so, and one given up on too early leaves the user
// believing it was sent.
func TestPermanenceIsClassifiedFromTheReplyCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nothing went wrong", nil, false},
		{"our own refusal", fmt.Errorf("wrapped: %w", ErrRefused), true},
		{"a 5xx refusal", &smtp.SMTPError{Code: 550, Message: "no such user"}, true},
		{"a 4xx deferral", &smtp.SMTPError{Code: 451, Message: "try later"}, false},
		{"a dropped connection", errors.New("connection reset by peer"), false},
		// Anything unrecognised is transient, because giving up on a message
		// the server never refused is the worse of the two mistakes.
		{"something unfamiliar", errors.New("who knows"), false},
	}

	for _, c := range cases {
		if got := IsPermanent(c.err); got != c.want {
			t.Errorf("%s: IsPermanent() = %v, want %v", c.name, got, c.want)
		}
	}
}

// The refusals made before the transaction have to be the permanent kind, or
// the queue retries a message that can never be accepted as it is.
func TestLocalRefusalsAreMarkedPermanent(t *testing.T) {
	c := &Client{caps: Capabilities{MaxMessageSize: 10}}
	env := Envelope{From: "u@example.com", To: []string{"r@example.com"}}

	err := c.checkAcceptable(env, []byte(strings.Repeat("x", 100)))
	if err == nil {
		t.Fatal("an over-large message was accepted")
	}
	if !IsPermanent(err) {
		t.Errorf("a size refusal was classified as worth retrying: %v", err)
	}
}

// An OAuth account could read mail and could not send any: submission had its
// own credential path that took a password and refused anything else.
func TestATokenAccountCanSend(t *testing.T) {
	cfg, got := startServer(t, serverOptions{
		authMechs: []string{"XOAUTH2"},
		username:  "u@example.com",
		token:     "ya29.TOKEN",
	})

	c := dialWith(t, cfg, tokenCredential{username: "u@example.com", token: "ya29.TOKEN"})

	env := Envelope{From: "u@example.com", To: []string{"r@example.com"}}
	if err := c.Send(context.Background(), env, message("Konu", "metin")); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if mech := got.snapshot().mechanism; mech != "XOAUTH2" {
		t.Errorf("the server saw mechanism %q", mech)
	}
}

// A server that will not take a token says so in those words. The alternative
// is an opaque SASL failure on an account that is correctly configured
// everywhere else.
func TestATokenAccountAgainstAPasswordOnlyServerIsExplained(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{authMechs: []string{"PLAIN", "LOGIN"}})

	_, err := Dial(context.Background(), cfg,
		tokenCredential{username: "u@example.com", token: "ya29.TOKEN"})
	if err == nil {
		t.Fatal("Dial() claimed to authenticate with a token the server cannot take")
	}
	if !strings.Contains(err.Error(), "signs in with a token") {
		t.Errorf("the error does not say what went wrong: %v", err)
	}
}

// Minting a token costs a network round trip and can fail. It must not read as
// a rejected credential, which is what somebody would go and re-enter.
func TestAFailureToMintATokenSaysSo(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{
		authMechs: []string{"XOAUTH2"}, username: "u@example.com", token: "t",
	})

	_, err := Dial(context.Background(), cfg, tokenCredential{
		username: "u@example.com", err: errors.New("the refresh token has been revoked"),
	})
	if err == nil {
		t.Fatal("Dial() succeeded with no token")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("the error lost the reason: %v", err)
	}
}

// The secret leaves the keyring only once there is a mechanism that can carry
// it. Reading it first would put a password in this process to talk to a
// server that was never going to accept one.
func TestThePasswordIsNotReadWhenNoMechanismFits(t *testing.T) {
	cfg, _ := startServer(t, serverOptions{authMechs: []string{"CRAM-MD5"}})
	credential := &passwordCredential{username: "u@example.com", secret: "pw"}

	if _, err := Dial(context.Background(), cfg, credential); err == nil {
		t.Fatal("Dial() accepted a server offering nothing this client speaks")
	}
	if credential.reads != 0 {
		t.Errorf("the password was read %d times for a server that cannot take it",
			credential.reads)
	}
}
