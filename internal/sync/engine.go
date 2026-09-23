// Package sync keeps the local database in step with mail servers.
//
// It talks to servers through imapx.MailBackend and never imports go-imap —
// depguard enforces that. This is what makes the engine testable against a
// scriptable fake instead of a live account.
package sync

import (
	"context"
	"sync"
	"time"

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
	ClaimOperations(ctx context.Context, accountID int64, limit int) ([]model.Operation, error)
	MarkOperationDone(ctx context.Context, id int64) error
	MarkOperationDropped(ctx context.Context, id int64) error
	MarkOperationFailed(ctx context.Context, id int64, reason string, retryAt time.Time) error
	MarkOperationPermanentlyFailed(ctx context.Context, id int64, reason string) error
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

	// nudges lets the UI wake a watch loop that is sitting in IDLE. Without
	// it a queued change waits for the server to happen to say something,
	// which on a quiet mailbox can be hours.
	nudgeMu sync.Mutex
	nudges  map[int64]chan struct{}
}

func New(s Store, d Dialer) *Engine {
	return &Engine{
		store:     s,
		dial:      d,
		retention: model.DefaultRetention,
		nudges:    map[int64]chan struct{}{},
	}
}

// Nudge asks the watch loop for an account to stop waiting and drain the queue
// now. Safe to call for an account nobody is watching, which is the normal
// case while a connection is down.
func (e *Engine) Nudge(accountID int64) {
	e.nudgeMu.Lock()
	ch, ok := e.nudges[accountID]
	e.nudgeMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- struct{}{}:
	default: // a wake-up is already pending; one is enough
	}
}

// registerNudges gives one watch loop a channel to be woken through, and
// returns the function that takes it away again.
func (e *Engine) registerNudges(accountID int64) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)

	e.nudgeMu.Lock()
	e.nudges[accountID] = ch
	e.nudgeMu.Unlock()

	return ch, func() {
		e.nudgeMu.Lock()
		if e.nudges[accountID] == ch {
			delete(e.nudges, accountID)
		}
		e.nudgeMu.Unlock()
	}
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
