// Package smtpx wraps go-smtp so the rest of the app sends mail through one
// narrow interface.
//
// The same shape as imapx, for the same reasons: the outgoing queue depends on
// MailSender, never on go-smtp directly, and depguard enforces it. That is
// what lets sending be tested against an in-memory server instead of a live
// account, and it leaves room for a second transport — Microsoft Graph sends
// mail over HTTP, not SMTP — without touching the queue.
package smtpx

import (
	"context"

	"nexusmail/internal/model"
)

// Config describes how to reach one submission server.
type Config struct {
	Host string
	Port int
	// Security is how the connection is encrypted. The zero value is implicit
	// TLS, so a caller that forgets to set it gets the safe answer.
	Security model.ConnectionSecurity
	// Insecure talks to the server in the clear, with no upgrade.
	//
	// Only tests set this, against a loopback server, and smtpx refuses to
	// send a password over such a connection regardless — see checkSecurity.
	// It exists because the in-memory server the queue is tested against
	// speaks no TLS, not because plaintext is a configuration anyone should
	// have.
	Insecure bool
	// Username is the address sent in the SASL exchange.
	Username string
}

// Capabilities records what the connected server actually offered.
//
// Recorded rather than assumed, because every one of these changes what may be
// put on the wire. A message with a UTF-8 address sent to a server with no
// SMTPUTF8 is not a message that arrives mangled; it is a message the server
// rejects, and the reader deserves to be told why before it is queued rather
// than after it has failed eight times.
type Capabilities struct {
	// StartTLS was advertised. Recorded even after the upgrade, because the
	// absence of it on a port that should have had it is worth reporting.
	StartTLS bool
	// EightBitMIME allows raw 8-bit bytes in the body. Without it the body has
	// to be transfer-encoded to 7 bits.
	EightBitMIME bool
	// SMTPUTF8 allows non-ASCII in the envelope addresses themselves.
	SMTPUTF8 bool
	// MaxMessageSize is the SIZE the server declared, or zero when it declared
	// none. Zero means unknown, not unlimited.
	MaxMessageSize int64
	// Auth lists the mechanisms the server named, upper-cased.
	Auth []string
}

// Envelope is who a message is from and where it goes, as the transport sees
// it — which is not the same as the From and To headers inside the message.
//
// A bcc recipient is in the envelope and in no header, and a mailing list
// posts to addresses that appear nowhere in the message at all. Keeping the
// two apart here is what makes both possible.
type Envelope struct {
	// From is the return path. Bounces come back to it.
	From string
	// To is every recipient, visible or not.
	To []string
}

// MailSender submits one message.
//
// Deliberately small. Everything else a mail client does with an outgoing
// message — saving a copy to Sent, recording it in the queue, attaching files
// — happens above this, because none of it is the transport's business.
type MailSender interface {
	// Capabilities reports what the server advertised after authentication.
	Capabilities() Capabilities

	// Send submits one already-assembled message.
	//
	// raw is the complete RFC 5322 message, headers and all. This layer does
	// not build messages: composing is a question about the user's intent and
	// belongs where that intent is known.
	Send(ctx context.Context, env Envelope, raw []byte) error

	Close() error
}
