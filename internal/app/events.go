package app

// Event names shared with the frontend. Keep them in step with
// frontend/src/lib/events.ts — there are three, so duplicating them is
// cheaper than generating them. If this list grows, generate it.
const (
	EventSyncStarted  = "sync:started"
	EventSyncFinished = "sync:finished"
	EventSyncFailed   = "sync:failed"
)

// SyncEvent is the payload for all three sync events.
type SyncEvent struct {
	AccountID int64  `json:"accountId"`
	Email     string `json:"email"`
	// Error is empty except on sync:failed.
	Error string `json:"error,omitempty"`
	// Class is the error class name: transient, auth, protocol or permanent.
	// The UI branches on it — an auth failure needs a sign-in prompt, not the
	// spinner a transient one gets.
	Class string `json:"class,omitempty"`
}

// Emitter delivers events to the frontend.
//
// main.go supplies a function backed by the Wails runtime; tests supply a
// recorder. This indirection is why internal/app does not import Wails at all,
// and therefore why every service method is unit-testable without a window.
type Emitter func(name string, data any)
