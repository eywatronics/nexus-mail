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
	"time"

	"nexusmail/internal/model"
)

// Config describes how to reach one server.
type Config struct {
	Host string
	Port int
	// Security is how the connection is encrypted. The zero value is implicit
	// TLS, so a caller that forgets to set it gets the safe answer.
	Security model.ConnectionSecurity
	// Insecure talks to the server in the clear, with no upgrade.
	//
	// Only tests set this, against a loopback server, and imapx refuses to
	// send a password over such a connection regardless — see Dial. It exists
	// because the in-memory server the sync engine is tested against speaks no
	// TLS, not because plaintext is a configuration anyone should have.
	Insecure bool
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
	// UIDPlus is what makes a scoped expunge possible. Without it the only
	// EXPUNGE available removes every message in the mailbox flagged deleted,
	// including ones another client flagged.
	UIDPlus bool
	// SpecialUse means the server will say which mailbox is Sent, Drafts and
	// so on, instead of leaving the client to guess from the name.
	SpecialUse bool
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

	// FetchPart returns the decoded bytes of one part of a message, which is
	// how an attachment is downloaded. Separate from FetchBody because a
	// twenty-megabyte file is not wanted until somebody asks for it.
	FetchPart(ctx context.Context, uid uint32, partID, encoding string) ([]byte, error)

	// StoreFlags adds or removes flags on a set of UIDs in the selected
	// mailbox. Adding a flag that is already set is harmless, which is what
	// makes a retry safe.
	StoreFlags(ctx context.Context, uids []uint32, flags []string, add bool) error

	// Move relocates messages from the selected mailbox to another one.
	Move(ctx context.Context, uids []uint32, destPath string) error

	// Expunge permanently removes messages from the selected mailbox.
	Expunge(ctx context.Context, uids []uint32) error

	// EmptyFolder destroys every message in the selected mailbox, including
	// the ones this client never downloaded.
	EmptyFolder(ctx context.Context) error

	// Idle waits for the server to report that the selected mailbox changed,
	// returning true when it did. It blocks until then or until ctx is done.
	Idle(ctx context.Context) (bool, error)

	// FetchFlags returns UIDs and their current flags for a range, without
	// envelopes or bodies. This is the cheap half of a delta sync.
	//
	// changedSince is a CONDSTORE modification sequence: non-zero asks the
	// server for only what changed after it, which turns a scan of a large
	// mailbox into a short answer. Zero means "everything in range" — the
	// fallback for servers with no CONDSTORE, where the returned UID list
	// doubles as the set of messages that still exist.
	FetchFlags(ctx context.Context, r UIDRange, changedSince uint64) ([]model.FlagUpdate, error)

	// FetchBody returns the HTML and plain-text parts of one message.
	FetchBody(ctx context.Context, uid uint32) (Body, error)

	// FetchRaw returns the message as it arrived, headers and all. It is what
	// "view source", "save as .eml" and the charset repair all work from.
	FetchRaw(ctx context.Context, uid uint32) ([]byte, error)

	// Append writes a message into a mailbox, which is how a sent copy is
	// filed. Sending happens over SMTP and leaves no trace in the mailbox, so
	// without this a message the user sent would exist on the recipient's
	// server and nowhere they could see it.
	//
	// The returned UID is zero when the server does not offer UIDPLUS: filed,
	// but we do not know where. The next sync of that folder finds it.
	Append(ctx context.Context, mailbox string, raw []byte, flags []string,
		when time.Time) (uint32, error)

	Close() error
}
