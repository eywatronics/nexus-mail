// Package store owns all SQLite persistence for Nexus Mail.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store holds two connection pools over the same database file.
//
// The design doc calls for "a single writer goroutine plus a read pool". This
// gets the same guarantee more cheaply: a write pool capped at one connection
// serialises writes through database/sql itself, with no channel plumbing to
// write or to test. Reads go to a separate pool and run concurrently under WAL.
//
// The rule that follows from it: every INSERT, UPDATE and DELETE must go
// through Write(). Using Read() for a write reintroduces the lock contention
// this design exists to avoid, which concurrency_test.go asserts against.
type Store struct {
	read  *sql.DB
	write *sql.DB
}

// readPoolSize is deliberately modest. The UI issues a handful of concurrent
// reads at most; a large pool would only add file handles.
const readPoolSize = 4

const dsnOptions = "?_pragma=journal_mode(WAL)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=foreign_keys(1)" +
	"&_pragma=synchronous(NORMAL)"

// Open opens (creating if needed) mail.db inside dir and applies migrations.
func Open(dir string) (*Store, error) {
	dsn := "file:" + filepath.ToSlash(filepath.Join(dir, "mail.db")) + dsnOptions

	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open write pool: %w", err)
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = write.Close()
		return nil, fmt.Errorf("store: open read pool: %w", err)
	}
	read.SetMaxOpenConns(readPoolSize)

	s := &Store{read: read, write: write}

	if err := migrate(s.write); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return s, nil
}

// Read returns the concurrent read pool.
func (s *Store) Read() *sql.DB { return s.read }

// Write returns the serialised write pool. All mutations must use it.
func (s *Store) Write() *sql.DB { return s.write }

// Close closes both pools, reporting the write pool's error in preference
// since a failure there is the one that can lose data.
func (s *Store) Close() error {
	rerr := s.read.Close()
	werr := s.write.Close()
	if werr != nil {
		return werr
	}
	return rerr
}
