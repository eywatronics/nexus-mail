// Package model holds the data types shared across layers. It imports nothing
// from the rest of the project, so every layer can depend on it freely —
// depguard enforces that.
package model

import (
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
	BodyFetched    bool
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
