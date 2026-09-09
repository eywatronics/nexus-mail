package auth

import (
	"sync/atomic"
	"time"
)

// pendingUnlock records that a keyring call is stuck waiting on the OS.
//
// A D-Bus or Keychain call cannot be cancelled from Go: on timeout the
// goroutine making it stays alive until the OS prompt is dismissed. Leaking
// one goroutine that way is acceptable; leaking one per sync cycle is not. So
// while a call is stuck, further calls fail fast instead of stacking up.
var pendingUnlock atomic.Bool

// withTimeout runs fn on its own goroutine and gives up after keyringTimeout,
// returning ErrKeyringLocked.
func withTimeout[T any](fn func() (T, error)) (T, error) {
	var zero T

	if pendingUnlock.Load() {
		return zero, ErrKeyringLocked
	}

	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)

	pendingUnlock.Store(true)
	go func() {
		v, err := fn()
		// Clearing the flag before sending keeps a fast call from leaving the
		// gate shut: the receiver may already have given up.
		pendingUnlock.Store(false)
		done <- result{v, err}
	}()

	select {
	case r := <-done:
		return r.value, r.err
	case <-time.After(keyringTimeout):
		// pendingUnlock stays true until the abandoned goroutine finishes,
		// which is what makes subsequent calls fail fast.
		return zero, ErrKeyringLocked
	}
}

// withTimeoutVoid adapts withTimeout for calls that return only an error.
func withTimeoutVoid(fn func() error) error {
	_, err := withTimeout(func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}
