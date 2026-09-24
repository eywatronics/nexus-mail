package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Outbox holds messages waiting to be sent.
//
// Files rather than rows. Until a message is sent this file is the only copy
// in existence, and a twenty-megabyte attachment is not a thing to put in a
// text column: every read of the operations table would carry it, and SQLite
// would rewrite the whole row on each retry.
//
// The queue row holds only the name. Where the directory lives is the host's
// business, and a stored absolute path would break the first time the
// application moved or the profile was copied to another machine.
type Outbox struct {
	dir string
}

// NewOutbox uses an existing directory. The caller creates it — paths.OutboxDir
// does — so that the one place that knows where application data belongs stays
// the one place that knows.
func NewOutbox(dir string) *Outbox { return &Outbox{dir: dir} }

// Put writes a message and returns the name to record in the queue.
//
// Written to a temporary neighbour and renamed. A crash halfway through a
// twenty-megabyte write would otherwise leave a truncated file that the worker
// would happily submit: a message delivered with its last page missing is
// worse than one that was never sent, because nobody knows to send it again.
func (o *Outbox) Put(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("store: refusing to queue an empty message")
	}

	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("store: naming an outbox file: %w", err)
	}
	name := hex.EncodeToString(buf[:]) + ".eml"

	final := filepath.Join(o.dir, name)
	temp := final + ".tmp"

	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return "", fmt.Errorf("store: writing the outbox file: %w", err)
	}
	if err := os.Rename(temp, final); err != nil {
		_ = os.Remove(temp)
		return "", fmt.Errorf("store: putting the message in the outbox: %w", err)
	}
	return name, nil
}

// Get reads a queued message back.
func (o *Outbox) Get(name string) ([]byte, error) {
	path, err := o.resolve(name)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("store: reading the outbox file: %w", err)
	}
	return raw, nil
}

// Remove deletes a message that has gone out.
//
// A missing file is not an error. The worker deletes after the server accepted
// the message, and a retry that reaches this point twice has already done the
// only thing that mattered.
func (o *Outbox) Remove(name string) error {
	path, err := o.resolve(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: removing the outbox file: %w", err)
	}
	return nil
}

// resolve turns a recorded name into a path inside the outbox, and refuses
// anything that is not one.
//
// The name comes out of the database, which makes it data this code did not
// write in this process. A row saying "../../.ssh/id_rsa" would otherwise turn
// Get into a file read of the caller's choosing and Remove into a delete —
// and the queue is exactly the sort of place a stale or hand-edited row turns
// up.
func (o *Outbox) resolve(name string) (string, error) {
	if name == "" {
		return "", errors.New("store: an outbox reference cannot be empty")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("store: %q is not an outbox file name", name)
	}
	if !strings.HasSuffix(name, ".eml") {
		return "", fmt.Errorf("store: %q is not an outbox file name", name)
	}
	return filepath.Join(o.dir, name), nil
}

// Sweep removes files no queue row refers to.
//
// They accumulate two ways: a crash between writing the file and recording the
// row, and an operation the user cancelled. Neither is common and both leave a
// file nothing will ever read, which on a mailbox with large attachments is
// not a rounding error.
//
// Called with the names still in use. Deciding that here from the database
// would mean this file knew about the queue, and the whole point of the split
// is that it does not.
func (o *Outbox) Sweep(inUse []string) (int, error) {
	keep := make(map[string]bool, len(inUse))
	for _, name := range inUse {
		keep[name] = true
	}

	entries, err := os.ReadDir(o.dir)
	if err != nil {
		return 0, fmt.Errorf("store: reading the outbox: %w", err)
	}

	removed := 0
	for _, e := range entries {
		if e.IsDir() || keep[e.Name()] {
			continue
		}
		// Temporary files from an interrupted Put go too: a rename that never
		// happened left bytes nothing refers to.
		if !strings.HasSuffix(e.Name(), ".eml") && !strings.HasSuffix(e.Name(), ".eml.tmp") {
			continue
		}
		if err := os.Remove(filepath.Join(o.dir, e.Name())); err != nil {
			return removed, fmt.Errorf("store: sweeping the outbox: %w", err)
		}
		removed++
	}
	return removed, nil
}

// PendingOutboxNames is every outbox file a queue row still refers to.
func (s *Store) PendingOutboxNames() ([]string, error) {
	rows, err := s.read.Query(
		`SELECT json_extract(payload, '$.outbox') FROM operations
		  WHERE kind = 'send' AND state IN ('pending', 'failed')`)
	if err != nil {
		return nil, fmt.Errorf("store: listing queued messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var name *string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name != nil && *name != "" {
			out = append(out, *name)
		}
	}
	return out, rows.Err()
}
