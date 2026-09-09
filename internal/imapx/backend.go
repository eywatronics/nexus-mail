// Package imapx wraps go-imap so the rest of the app talks to mail servers
// through one narrow interface.
//
// The sync engine depends on MailBackend, never on go-imap directly — depguard
// enforces that. It is what lets the engine be tested against an in-memory
// server instead of a live account, and it leaves room for a second protocol
// later without touching the engine.
package imapx

import (
	"context"

	"nexusmail/internal/model"
)

// Config describes how to reach one server.
type Config struct {
	Host string
	Port int
	// TLS selects implicit TLS on connect (port 993). Tests set it false to
	// talk to a plaintext loopback server; production always sets it true.
	TLS bool
	// Username is the address sent in the SASL exchange.
	Username string
}

// Capabilities records what the connected server actually supports, so callers
// pick a strategy instead of assuming one. Not every server advertises
// CONDSTORE, and a delta sync that assumes it silently misses changes.
type Capabilities struct {
	CondStore bool
	QResync   bool
	Move      bool
	Idle      bool
}

// SelectResult carries the mailbox state a SELECT reports. HighestModSeq is
// zero when the server does not advertise CONDSTORE.
type SelectResult struct {
	UIDValidity   uint32
	UIDNext       uint32
	NumMessages   uint32
	HighestModSeq uint64
}

// UIDRange is an inclusive UID range. End of 0 means "through the highest UID".
type UIDRange struct {
	Start uint32
	End   uint32
}

// Body holds the two renderable representations of a message.
type Body struct {
	HTML string
	Text string
}

// MailBackend is one authenticated connection to one account.
//
// Implementations are NOT safe for concurrent use: IMAP is a stateful,
// sequential protocol and the selected mailbox is a property of the
// connection. The sync engine owns one backend per connection and never shares
// it across goroutines.
type MailBackend interface {
	// Capabilities reports what the server advertised after authentication.
	Capabilities() Capabilities

	// ListFolders returns every mailbox with its status counts filled in.
	ListFolders(ctx context.Context) ([]model.Folder, error)

	// Select opens a mailbox for subsequent fetches.
	Select(ctx context.Context, path string) (SelectResult, error)

	// FetchHeaders returns message headers for a UID range in the selected
	// mailbox. Bodies are deliberately not fetched.
	FetchHeaders(ctx context.Context, r UIDRange) ([]model.Message, error)

	// FetchBody returns the HTML and plain-text parts of one message.
	FetchBody(ctx context.Context, uid uint32) (Body, error)

	Close() error
}
