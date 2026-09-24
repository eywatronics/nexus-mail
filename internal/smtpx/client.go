package smtpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"nexusmail/internal/model"
)

// Client is one authenticated submission connection.
type Client struct {
	c    *smtp.Client
	caps Capabilities
}

var _ MailSender = (*Client)(nil)

// ehloName is what the client calls itself in EHLO.
//
// Deliberately not the machine's hostname, which is what most clients send and
// what go-smtp defaults to. A hostname is frequently a person's name and their
// employer, it travels in the Received headers of every message they send, and
// the server has their address anyway. "localhost" tells the server nothing it
// needs and nothing it does not.
const ehloName = "localhost"

// Dial opens a connection, upgrades it if asked, and authenticates.
//
// An empty secret means no authentication at all, which is only reachable on a
// loopback server: checkSecurity refuses cleartext elsewhere, and a TLS server
// wanting no credentials is not a configuration this client offers.
func Dial(ctx context.Context, cfg Config, secret string) (*Client, error) {
	if err := checkSecurity(cfg); err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	c, err := connect(ctx, addr, cfg)
	if err != nil {
		return nil, describeDialError(cfg, err)
	}

	if err := c.Hello(ehloName); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("smtpx: greeting %s: %w", cfg.Host, err)
	}

	client := &Client{c: c, caps: readCapabilities(c)}

	if secret != "" {
		if err := client.authenticate(cfg.Username, secret); err != nil {
			_ = c.Close()
			return nil, err
		}
		// Re-read: a server may advertise more once authenticated, and SIZE in
		// particular is often only accurate then.
		client.caps = readCapabilities(c)
	}
	return client, nil
}

func (c *Client) Capabilities() Capabilities { return c.caps }

func (c *Client) Close() error {
	// Quit is the polite close and it can fail on a connection the server has
	// already dropped. Either way the socket goes.
	if err := c.c.Quit(); err != nil {
		return c.c.Close()
	}
	return nil
}

// checkSecurity refuses a cleartext connection to anything but loopback.
//
// The same rule as imapx, written out again rather than shared: the two
// packages must not depend on each other, and a copied guard is cheaper than
// the shared layer that would let one be dropped from a distance.
func checkSecurity(cfg Config) error {
	if !cfg.Insecure {
		return nil
	}
	if isLoopback(cfg.Host) {
		return nil
	}
	return fmt.Errorf(
		"smtpx: refusing an unencrypted connection to %q; plaintext is for the "+
			"loopback test server and nothing else", cfg.Host)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// connect opens the transport the configuration asks for.
//
// STARTTLS is mandatory rather than opportunistic, for the reason it is in
// imapx: an attacker who can strip the advertisement from the greeting would
// otherwise be handed the password, and that attack is why opportunistic
// encryption is not encryption.
func connect(ctx context.Context, addr string, cfg Config) (*smtp.Client, error) {
	dialer := &net.Dialer{}
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

	switch {
	case cfg.Insecure:
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn), nil

	case cfg.Security == model.SecuritySTARTTLS:
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, err
		}
		upgraded, err := smtp.NewClientStartTLS(conn, tlsCfg)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return upgraded, nil

	default:
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn), nil
	}
}

// describeDialError names the two mistakes that produce an unreadable error.
//
// Implicit TLS on 587 and STARTTLS on 465 both fail deep inside the handshake,
// and the message a user sees is about a record header or an unexpected EOF.
// Neither says "you picked the wrong one", which is what happened.
func describeDialError(cfg Config, err error) error {
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.As(err, &certErr):
		return fmt.Errorf("smtpx: %s presented a certificate this machine does not trust: %w",
			cfg.Host, err)
	case cfg.Security != model.SecuritySTARTTLS && cfg.Port == 587:
		return fmt.Errorf("smtpx: port 587 is the STARTTLS submission port, but this "+
			"account is set to implicit TLS: %w", err)
	case cfg.Security == model.SecuritySTARTTLS && cfg.Port == 465:
		return fmt.Errorf("smtpx: port 465 is the implicit-TLS submission port, but this "+
			"account is set to STARTTLS: %w", err)
	default:
		return fmt.Errorf("smtpx: connecting to %s: %w",
			net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), err)
	}
}

func readCapabilities(c *smtp.Client) Capabilities {
	caps := Capabilities{}
	caps.StartTLS, _ = c.Extension("STARTTLS")
	caps.EightBitMIME, _ = c.Extension("8BITMIME")
	caps.SMTPUTF8, _ = c.Extension("SMTPUTF8")

	if ok, param := c.Extension("SIZE"); ok {
		// The parameter is optional: "SIZE" on its own means the server takes
		// the extension without declaring a limit. Left at zero, which means
		// unknown rather than unlimited.
		if n, err := strconv.ParseInt(strings.TrimSpace(param), 10, 64); err == nil && n > 0 {
			caps.MaxMessageSize = n
		}
	}
	if ok, param := c.Extension("AUTH"); ok {
		caps.Auth = strings.Fields(strings.ToUpper(param))
	}
	return caps
}

// authenticate picks a mechanism from what the server named.
//
// PLAIN first, then LOGIN, which is the order imapx uses and for the same
// reason: PLAIN is the one every server implements the same way, and LOGIN is
// the older thing to fall back to. Both send the secret, which is why neither
// is reachable without encryption.
func (c *Client) authenticate(username, secret string) error {
	switch {
	case c.offers("PLAIN"):
		return c.wrapAuth(c.c.Auth(sasl.NewPlainClient("", username, secret)))
	case c.offers("LOGIN"):
		return c.wrapAuth(c.c.Auth(sasl.NewLoginClient(username, secret)))
	case len(c.caps.Auth) == 0:
		return errors.New("smtpx: this server advertised no authentication mechanisms; " +
			"it may not be a submission port")
	default:
		return fmt.Errorf("smtpx: this server offers only %s, none of which this client "+
			"can use", strings.Join(c.caps.Auth, ", "))
	}
}

func (c *Client) offers(mechanism string) bool {
	return slices.Contains(c.caps.Auth, mechanism)
}

func (c *Client) wrapAuth(err error) error {
	if err == nil {
		return nil
	}
	// The secret is never in the message. A failed login is reported by the
	// server with a code and a phrase, and neither needs the password beside
	// it to be understood.
	return fmt.Errorf("smtpx: the server rejected these credentials: %w", err)
}

// Send submits one already-assembled message.
func (c *Client) Send(ctx context.Context, env Envelope, raw []byte) error {
	if env.From == "" {
		return errors.New("smtpx: a message needs a return path")
	}
	if len(env.To) == 0 {
		return errors.New("smtpx: a message needs at least one recipient")
	}
	if err := c.checkAcceptable(env, raw); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	opts := &smtp.MailOptions{Size: int64(len(raw))}
	if c.caps.EightBitMIME {
		opts.Body = smtp.Body8BitMIME
	}
	if c.caps.SMTPUTF8 && !envelopeIsASCII(env) {
		opts.UTF8 = true
	}

	if err := c.c.Mail(env.From, opts); err != nil {
		return fmt.Errorf("smtpx: the server refused the sender: %w", err)
	}
	for _, to := range env.To {
		if err := c.c.Rcpt(to, nil); err != nil {
			return fmt.Errorf("smtpx: the server refused a recipient: %w", err)
		}
	}

	w, err := c.c.Data()
	if err != nil {
		return fmt.Errorf("smtpx: the server refused to take the message: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtpx: sending the message: %w", err)
	}
	// The close is the send. A write that succeeded and a close that failed is
	// a message the server did not accept, and reporting success there would
	// leave the user believing a mail went out.
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtpx: the server did not accept the message: %w", err)
	}
	return nil
}

// checkAcceptable refuses, before the upload, what the server would refuse
// after it.
//
// The cost of not doing this is paid twice. A message over the declared SIZE
// is rejected only once the whole body has crossed the wire, which on a slow
// connection with a large attachment is minutes spent to be told no — and the
// queue, seeing a server error, would spend them again on the retry.
func (c *Client) checkAcceptable(env Envelope, raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("%w: it is empty", ErrRefused)
	}
	if c.caps.MaxMessageSize > 0 && int64(len(raw)) > c.caps.MaxMessageSize {
		return fmt.Errorf("%w: it is %d bytes and the server accepts at most %d",
			ErrRefused, len(raw), c.caps.MaxMessageSize)
	}
	if !c.caps.EightBitMIME && !bytesAreASCII(raw) {
		return fmt.Errorf("%w: it has 8-bit content and the server does not offer "+
			"8BITMIME, so it has to be transfer-encoded to 7 bits first", ErrRefused)
	}
	if !c.caps.SMTPUTF8 && !envelopeIsASCII(env) {
		return fmt.Errorf("%w: an address on it is not ASCII and the server does not "+
			"offer SMTPUTF8", ErrRefused)
	}
	return nil
}

func envelopeIsASCII(env Envelope) bool {
	if !stringIsASCII(env.From) {
		return false
	}
	for _, to := range env.To {
		if !stringIsASCII(to) {
			return false
		}
	}
	return true
}

func stringIsASCII(s string) bool { return utf8.RuneCountInString(s) == len(s) }

func bytesAreASCII(b []byte) bool {
	for _, c := range b {
		if c > 127 {
			return false
		}
	}
	return true
}

// ErrRefused marks a refusal this client made on the server's behalf, before
// the transaction started.
//
// Wrapped into the errors from checkAcceptable so the queue can tell them
// apart from a connection that dropped. A message over the declared SIZE or
// carrying 8-bit content a 7-bit server will not take is not going to succeed
// on the fourth attempt either, and retrying it every few minutes is how a
// message never leaves and never says so.
var ErrRefused = errors.New("smtpx: this message cannot be submitted as it is")

// IsPermanent reports whether retrying could ever help.
//
// The classification lives here because this is where the knowledge is: the
// SMTP reply code says it (5xx is a refusal, 4xx is "not now"), and the
// refusals this package makes itself say it by wrapping ErrRefused. A caller
// that had to know either would be a caller that had to import go-smtp, which
// is the dependency the MailSender interface exists to avoid.
//
// Anything unrecognised is transient. Giving up on a message the server never
// actually refused is the worse of the two mistakes: the user believes it was
// sent.
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRefused) {
		return true
	}

	var smtpErr *smtp.SMTPError
	if errors.As(err, &smtpErr) {
		return !smtpErr.Temporary()
	}
	return false
}
