// Package sync keeps the local database in step with mail servers.
//
// It talks to servers through imapx.MailBackend and never imports go-imap —
// depguard enforces that. This is what makes the engine testable against a
// scriptable fake instead of a live account.
package sync

import (
	"context"
	"sync"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// InitialHeaderCount is how many recent messages get headers on first sync.
//
// Folders beyond the inbox and the special ones are listed only; their headers
// arrive when the user first opens them. On a corporate account with dozens of
// folders, fetching everything up front would take minutes before the window
// showed anything.
const InitialHeaderCount = 1000

// Store is the persistence surface the engine needs. *store.Store satisfies
// it. Declaring the dependency as an interface here, rather than importing the
// concrete type, keeps the direction of the dependency visible.
type Store interface {
	UpsertFolders(ctx context.Context, accountID int64, folders []model.Folder) error
	ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error)
	ResetFolder(ctx context.Context, folderID int64, newUIDValidity uint32) error
	UpsertMessages(ctx context.Context, folderID int64, msgs []model.Message) error
	ListMessages(ctx context.Context, folderID int64, limit, offset int) ([]model.Message, error)
	ListMessageUIDs(ctx context.Context, folderID int64) ([]uint32, error)
	SetMessageFlags(ctx context.Context, folderID int64, updates []model.FlagUpdate) error
	DeleteMessagesByUID(ctx context.Context, folderID int64, uids []uint32) error
	PurgeFolder(ctx context.Context, folderID int64, policy model.RetentionPolicy) (int, error)
	SetMessageBody(ctx context.Context, messageID int64, html, text string) error
	GetMessageBody(ctx context.Context, messageID int64) (html, text string, err error)
}

// Dialer opens an authenticated connection for one account. The engine takes
// this as a function so tests can hand it a fake backend.
type Dialer func(ctx context.Context, accountID int64) (imapx.MailBackend, error)

// Engine performs synchronisation for all accounts.
type Engine struct {
	store Store
	dial  Dialer

	// retention bounds how much of each folder stays on disk. Guarded because
	// the watch loop reads it from its own goroutine while the settings screen
	// can write it.
	retentionMu sync.RWMutex
	retention   model.RetentionPolicy
}

func New(s Store, d Dialer) *Engine {
	return &Engine{store: s, dial: d, retention: model.DefaultRetention}
}

// SetRetention replaces the retention policy. A zero policy keeps everything,
// which is a setting the user is allowed to choose.
func (e *Engine) SetRetention(p model.RetentionPolicy) {
	e.retentionMu.Lock()
	defer e.retentionMu.Unlock()
	e.retention = p
}

func (e *Engine) retentionPolicy() model.RetentionPolicy {
	e.retentionMu.RLock()
	defer e.retentionMu.RUnlock()
	return e.retention
}
