// Package model holds the data types shared across layers. It imports nothing
// from the rest of the project, so every layer can depend on it freely —
// depguard enforces that.
package model

import (
	"net/mail"
	"sort"
	"strings"
	"time"
)

type AuthKind string

const (
	AuthPassword AuthKind = "password"
	AuthOAuth    AuthKind = "oauth"
)

type Provider string

const (
	ProviderMicrosoft Provider = "microsoft"
	ProviderGoogle    Provider = "google"
	ProviderGeneric   Provider = "generic"
)

// System flags as IMAP spells them. Servers vary in capitalisation, so compare
// with Message.HasFlag rather than with ==.
const (
	FlagSeen     = "\\Seen"
	FlagFlagged  = "\\Flagged"
	FlagDeleted  = "\\Deleted"
	FlagDraft    = "\\Draft"
	FlagAnswered = "\\Answered"
)

type Account struct {
	ID          int64
	Email       string
	DisplayName string
	Provider    Provider
	AuthKind    AuthKind
	IMAPHost    string
	IMAPPort    int
	SMTPHost    string
	SMTPPort    int
	// IMAPSecurity is how the connection is encrypted. Empty means SecurityTLS,
	// which is what every account created before this field existed used.
	IMAPSecurity ConnectionSecurity
	// SecretRef names the entry in the SecretStore. The secret itself is never
	// stored in the database.
	SecretRef string
	CreatedAt time.Time
}

// ConnectionSecurity is how a connection to a mail server is encrypted.
//
// There is no "none". A password sent in the clear is a password given away,
// and every mechanism this client can authenticate with sends something worth
// stealing. Offering the option would make a misconfiguration silent instead
// of impossible.
type ConnectionSecurity string

const (
	// SecurityTLS is implicit TLS: the connection is encrypted from the first
	// byte. Port 993. The default, and correct wherever it is offered.
	SecurityTLS ConnectionSecurity = "tls"
	// SecuritySTARTTLS opens in the clear and upgrades before authenticating.
	// Port 143.
	//
	// Worth supporting because on-premises Exchange is usually handed out this
	// way: its IMAP4 service defaults to LoginType SecureLogin, which requires
	// the upgrade before it will accept a password, and many deployments
	// publish only 143.
	//
	// The upgrade is mandatory, never opportunistic. A server that will not
	// upgrade is refused rather than talked to in the clear, because a network
	// attacker who can strip the STARTTLS advertisement gets the password.
	SecuritySTARTTLS ConnectionSecurity = "starttls"
)

// SecurityOrDefault reads a stored value, treating anything unrecognised as
// implicit TLS.
//
// Unrecognised includes empty, which is every account created before the
// column existed. Defaulting to the stronger option is the only safe direction
// to guess in.
func SecurityOrDefault(value string) ConnectionSecurity {
	if ConnectionSecurity(value) == SecuritySTARTTLS {
		return SecuritySTARTTLS
	}
	return SecurityTLS
}

// DefaultIMAPPort is the port that goes with a connection security, used when
// the user has not given one.
func DefaultIMAPPort(security ConnectionSecurity) int {
	if security == SecuritySTARTTLS {
		return 143
	}
	return 993
}

type Folder struct {
	ID         int64
	AccountID  int64
	Name       string
	Path       string
	Delimiter  string
	Attributes []string
	// UIDValidity changes when the server recreates the mailbox, invalidating
	// every UID we hold for it.
	UIDValidity uint32
	UIDNext     uint32
	// HighestModSeq is zero when the server does not advertise CONDSTORE.
	HighestModSeq uint64
	TotalCount    int
	UnreadCount   int
	LastSyncedAt  time.Time
}

// HasAttribute reports whether the folder carries an IMAP special-use
// attribute such as \Sent, case-insensitively.
func (f Folder) HasAttribute(attr string) bool {
	for _, got := range f.Attributes {
		if strings.EqualFold(got, attr) {
			return true
		}
	}
	return false
}

// IsInbox reports whether this is the account's inbox. INBOX is the one
// mailbox name IMAP defines as case-insensitive.
func (f Folder) IsInbox() bool {
	return strings.EqualFold(f.Path, "INBOX")
}

// FolderRole is what a mailbox is for, as opposed to what it is called.
//
// The name is the user's; the role is the client's. Sending needs to know
// which mailbox to file a copy in, and "Gönderilenler" is as much the sent
// folder as "Sent" is.
type FolderRole string

const (
	RoleNone    FolderRole = ""
	RoleInbox   FolderRole = "inbox"
	RoleDrafts  FolderRole = "drafts"
	RoleSent    FolderRole = "sent"
	RoleArchive FolderRole = "archive"
	RoleJunk    FolderRole = "junk"
	RoleTrash   FolderRole = "trash"
)

// specialUseRoles maps the RFC 6154 attributes to roles. \All and \Flagged
// are deliberately absent: they name Gmail's virtual mailboxes, which are
// views over mail that lives elsewhere, and treating one as a real folder is
// how a client ends up syncing every message twice.
// Keys are lowercase because the lookup lowercases what the server sent:
// servers are inconsistent about the case of flags, the same reason
// Message.HasFlag compares case-insensitively.
var specialUseRoles = map[string]FolderRole{
	"\\drafts":  RoleDrafts,
	"\\sent":    RoleSent,
	"\\archive": RoleArchive,
	"\\junk":    RoleJunk,
	"\\trash":   RoleTrash,
}

// roleNames are the folder names that mean a role on a server that does not
// advertise SPECIAL-USE.
//
// A guess, and only ever a fallback — the attribute is the authority. It is
// worth making because the servers without SPECIAL-USE are the old ones, and
// the old ones are exactly where a Turkish corporate account is likely to
// live. Matching is on a prefix of the lowercased leaf name, so "Sent Items"
// and "Gönderilmiş Öğeler" both land.
var roleNames = []struct {
	prefix string
	role   FolderRole
}{
	{"sent", RoleSent},
	{"gönder", RoleSent},
	{"gonder", RoleSent},
	{"draft", RoleDrafts},
	{"taslak", RoleDrafts},
	{"trash", RoleTrash},
	{"deleted", RoleTrash},
	{"çöp", RoleTrash},
	{"cop", RoleTrash},
	{"silin", RoleTrash},
	{"archive", RoleArchive},
	{"arşiv", RoleArchive},
	{"arsiv", RoleArchive},
	{"junk", RoleJunk},
	{"spam", RoleJunk},
	{"gereksiz", RoleJunk},
	{"istenmey", RoleJunk},
}

// Role reports what the mailbox is for.
//
// The special-use attribute wins whenever there is one: it is the server
// stating the answer rather than this code inferring it. The name is consulted
// only when there is no attribute at all.
func (f Folder) Role() FolderRole {
	if f.IsInbox() {
		return RoleInbox
	}
	for _, attr := range f.Attributes {
		if role, ok := specialUseRoles[strings.ToLower(attr)]; ok {
			return role
		}
	}

	name := strings.ToLower(f.Name)
	for _, candidate := range roleNames {
		if strings.HasPrefix(name, candidate.prefix) {
			return candidate.role
		}
	}
	return RoleNone
}

// roleOrder is the order roles are shown in. Mailboxes with no role follow,
// in the order the caller already had them.
var roleOrder = map[FolderRole]int{
	RoleInbox:   0,
	RoleDrafts:  1,
	RoleSent:    2,
	RoleArchive: 3,
	RoleJunk:    4,
	RoleTrash:   5,
}

// SortFolders puts the mailboxes that have a role first, in the order people
// expect to find them, and leaves the rest alone.
//
// Sorted here rather than in the window, because the order is a property of
// what the folders are, not of how they are drawn. Alphabetical order is
// actively unhelpful in a mail client: it scatters the six mailboxes used
// every day through a list of project folders, and on a Turkish account it
// does not even put them in the same places as on an English one.
func SortFolders(folders []Folder) {
	sort.SliceStable(folders, func(i, j int) bool {
		return folderRank(folders[i]) < folderRank(folders[j])
	})
}

func folderRank(f Folder) int {
	if rank, ok := roleOrder[f.Role()]; ok {
		return rank
	}
	return len(roleOrder)
}

type Address struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

// String is the address as it appears in a header.
//
// net/mail does the quoting, because the rules are not obvious: a display name
// containing a comma, a quote or a full stop has to be quoted or the address
// list parses as two addresses — and "Kabatepe, İsmet <u@example.com>" is
// exactly the shape a name takes in a corporate directory.
func (a Address) String() string {
	if a.Addr == "" {
		return a.Name
	}
	if a.Name == "" {
		return a.Addr
	}
	return (&mail.Address{Name: a.Name, Address: a.Addr}).String()
}

type Message struct {
	ID        int64
	AccountID int64
	FolderID  int64
	UID       uint32
	MessageID string
	ThreadID  string
	// ThreadCount is how many messages of this conversation are in this
	// folder. Filled in only by the reads that group by conversation; zero
	// everywhere else, which reads as "not asked".
	ThreadCount int
	InReplyTo   string
	References  []string
	Subject     string
	From        Address
	To          []Address
	Cc          []Address
	// ReplyTo is the Reply-To header, empty when the author did not set one.
	//
	// Kept beside From rather than folded into it on ingest, because the two
	// answer different questions: From is who wrote this, ReplyTo is where
	// they want the answer. A reader looking at a mailing list message needs
	// to be able to see both.
	ReplyTo []Address
	// Date comes from the Date: header, reconciled against InternalDate on
	// ingest: an unparseable or implausible header is replaced by the server's
	// delivery time. Sorting and display use InternalDate regardless.
	Date           time.Time
	InternalDate   time.Time
	Size           int64
	Snippet        string
	Flags          []string
	HasAttachments bool
	// Attachments is filled from the BODYSTRUCTURE at header-fetch time.
	Attachments []AttachmentPart
	BodyFetched bool
}

// AttachmentPart is one file carried by a message.
//
// Described from the BODYSTRUCTURE the header fetch already asks for, so
// listing a message's attachments costs nothing extra. Only the bytes need a
// second round trip, and only when somebody opens one.
type AttachmentPart struct {
	// ID is the local row id, zero for a part that has only been described by
	// the server and never stored.
	ID int64
	// PartID is the IMAP part number, such as "2" or "1.3". It is how the
	// bytes are fetched, and it is only meaningful together with the UID.
	PartID   string
	Filename string
	MIMEType string
	// Size is the encoded size the server reports, which is what it will send.
	Size int64
	// Encoding is the transfer encoding to undo, usually base64.
	Encoding string
	// LocalPath is where the bytes were saved, empty until somebody opens it.
	LocalPath  string
	Downloaded bool
}

// ActsOnUIDs reports whether a kind names the messages it applies to.
//
// Every kind but one does. The exception exists because "empty this folder"
// cannot be expressed as a list without becoming a different, weaker
// instruction — see OpEmptyFolder.
func (k OperationKind) ActsOnUIDs() bool {
	return k != OpEmptyFolder && k != OpSend
}

// NeedsFolder reports whether a kind is scoped to one mailbox.
//
// A send is not: the message has not reached a mailbox yet, and the folder it
// will eventually be copied into is decided after the server accepts it. The
// queue row therefore carries no folder and no UIDValidity stamp, and the
// worker must not check one.
func (k OperationKind) NeedsFolder() bool { return k != OpSend }

// OperationKind is what an outgoing operation asks the server to do.
//
// The set is deliberately small. Each one maps to a single IMAP command, so a
// worker never has to decide what a queued row "means" — only when to send it.
type OperationKind string

const (
	OpAddFlags    OperationKind = "add_flags"
	OpRemoveFlags OperationKind = "remove_flags"
	OpMove        OperationKind = "move"
	OpDelete      OperationKind = "delete"
	// OpEmptyFolder destroys everything in a mailbox, whether or not this
	// client ever synced it.
	//
	// It names no UIDs, and that is the point. Emptying the trash by listing
	// what we have would empty only the part we happened to have downloaded —
	// a folder of eight thousand messages with a hundred synced would look
	// emptied and would not be. The one honest way to say "everything" is to
	// let the server decide what everything means.
	OpEmptyFolder OperationKind = "empty_folder"

	// OpSend submits a message the user wrote.
	//
	// The odd one out, and worth saying why. Every other operation is an IMAP
	// command against a folder: it carries UIDs and a UIDValidity stamp, and
	// the worker applies it through a MailBackend. A send carries neither. It
	// needs an SMTP connection, it names no mailbox, and the message it
	// carries is too large for the queue row — so the row holds a reference to
	// a file in the outbox directory and the bytes live there until it goes.
	OpSend OperationKind = "send"
)

// OperationState tracks one queued change through its life.
//
// Dropped is not a failure. It means the server recreated the mailbox while
// the change was waiting, so the UIDs it names now refer to different
// messages; applying it would act on the wrong mail. See the UIDVALIDITY stamp
// in the design doc.
type OperationState string

const (
	OpPending OperationState = "pending"
	OpDone    OperationState = "done"
	OpFailed  OperationState = "failed"
	OpDropped OperationState = "dropped"
)

// Operation is one change waiting to reach the server.
//
// UIDValidity is the generation of the mailbox this was queued against. The
// worker compares it before sending: a mailbox the server has recreated has a
// new UID space, and the same number now names a different message.
type Operation struct {
	ID          int64
	AccountID   int64
	FolderID    int64
	UIDValidity uint32
	Kind        OperationKind
	State       OperationState

	// UIDs is what the operation acts on, in the folder it was queued against.
	UIDs []uint32
	// Flags carries the flags for OpAddFlags and OpRemoveFlags.
	Flags []string
	// TargetFolderID is where OpMove is going.
	TargetFolderID int64

	// Outbox names the file holding the raw message, for OpSend. A name
	// rather than a path: where the outbox directory lives is the host's
	// business, and a stored absolute path would break the first time the
	// application moved or the profile was copied to another machine.
	Outbox string
	// Envelope is who an OpSend message is from and where it goes, which is
	// not the same as its From and To headers — a blind copy is in the
	// envelope and in no header at all.
	EnvelopeFrom string
	EnvelopeTo   []string

	Attempts      int
	LastError     string
	CreatedAt     time.Time
	NextAttemptAt time.Time
}

// RetentionPolicy bounds how much of a folder is kept on disk.// RetentionPolicy bounds how much of a folder is kept on disk.
//
// IDLE running for months is what makes this necessary: the initial fetch is
// capped, but nothing caps growth afterwards. On a busy account that is
// thousands of new rows a year, forever.
//
// A message falls outside the window when it fails *either* limit — too old,
// or pushed out by newer mail. Zero on a field disables that limit; zero on
// both means keep everything, which is a setting the user is allowed to
// choose. On a local-first client the disk is theirs to spend.
type RetentionPolicy struct {
	// MaxAge is how far back to keep. Zero means no age limit.
	MaxAge time.Duration
	// MaxMessages is the per-folder cap. Zero means no count limit.
	MaxMessages int
}

// Enabled reports whether the policy would remove anything at all.
func (p RetentionPolicy) Enabled() bool {
	return p.MaxAge > 0 || p.MaxMessages > 0
}

// DefaultRetention is a year of mail, or twenty-five thousand messages per
// folder, whichever fills first. Generous enough that most people never notice
// it, small enough that the database does not grow without bound.
var DefaultRetention = RetentionPolicy{
	MaxAge:      365 * 24 * time.Hour,
	MaxMessages: 25_000,
}

// FlagUpdate is one message's UID and the flags the server now reports for it.
//
// Delta sync carries flags without headers: a message going from unread to
// read produces no new header data, and refetching the envelope to learn that
// would multiply the traffic the whole delta path exists to avoid.
type FlagUpdate struct {
	UID   uint32
	Flags []string
}

// HasFlag reports whether f is present, case-insensitively, since servers vary
// in how they capitalise system flags.
func (m Message) HasFlag(f string) bool {
	for _, got := range m.Flags {
		if strings.EqualFold(got, f) {
			return true
		}
	}
	return false
}

// Identity is who a message is from, which is not the same question as which
// account it was sent through.
//
// One account can have several: a person answers support@ and their own
// address from the same mailbox, and an alias is the ordinary way an employer
// hands somebody a second address. Each wants its own display name, reply-to
// and signature; none wants a second account with a second password.
type Identity struct {
	ID        int64
	AccountID int64

	Email       string
	DisplayName string
	// ReplyTo is where replies should go when that is not the From address.
	// Empty means the From address, which is what it means in the header too.
	ReplyTo string

	// SignatureText is always used. SignatureHTML is used only for an HTML
	// message, and a signature that exists only as HTML would read as a blank
	// space in anything that will not render it.
	SignatureText string
	SignatureHTML string

	// IsDefault marks the one the composer opens with. Exactly one per
	// account, which the schema enforces with a partial unique index rather
	// than leaving it to application code.
	IsDefault bool
	SortOrder int

	CreatedAt time.Time
}

// From is the identity as it appears in a From header.
func (i Identity) From() string {
	if i.DisplayName == "" {
		return i.Email
	}
	return i.DisplayName + " <" + i.Email + ">"
}
