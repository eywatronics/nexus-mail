// Package model holds the data types shared across layers. It imports nothing
// from the rest of the project, so every layer can depend on it freely —
// depguard enforces that.
package model

import (
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
	// SecretRef names the entry in the SecretStore. The secret itself is never
	// stored in the database.
	SecretRef string
	CreatedAt time.Time
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

type Message struct {
	ID         int64
	AccountID  int64
	FolderID   int64
	UID        uint32
	MessageID  string
	ThreadID   string
	InReplyTo  string
	References []string
	Subject    string
	From       Address
	To         []Address
	Cc         []Address
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
