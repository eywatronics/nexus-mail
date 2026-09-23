package app

// Event names shared with the frontend. Keep them in step with
// frontend/src/lib/events.ts — there are four, so duplicating them is
// cheaper than generating them. If this list grows, generate it.
const (
	EventSyncStarted  = "sync:started"
	EventSyncFinished = "sync:finished"
	EventSyncFailed   = "sync:failed"
	EventFindResults  = "find:results"
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

// FindEvent reports what a find in the reading pane turned up.
//
// An event rather than a method the window calls, because the counting happens
// in the same pass that builds the document: the frame asks for a body with a
// query on it, and the number of matches falls out of serving that request. A
// separate call would mean rendering the message twice and, worse, could
// disagree with what is on screen if the two renders ever saw different bodies.
type FindEvent struct {
	MessageID int64  `json:"messageId"`
	Query     string `json:"query"`
	Count     int    `json:"count"`
	// Current is the match the document actually scrolled to, after the index
	// the window asked for was wrapped into range.
	Current int `json:"current"`
}

// Emitter delivers events to the frontend.
//
// main.go supplies a function backed by the Wails runtime; tests supply a
// recorder. This indirection is why internal/app does not import Wails at all,
// and therefore why every service method is unit-testable without a window.
type Emitter func(name string, data any)
