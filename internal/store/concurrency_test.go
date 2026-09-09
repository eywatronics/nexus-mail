package store

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoLockContentionUnderLoad asserts that concurrent readers never observe
// SQLITE_BUSY while a writer works continuously.
//
// This needs its own test because `go test -race` cannot catch it: lock
// contention is not a data race, so the race detector is blind to it. Passing
// by leaning on busy_timeout alone is also not good enough — writes must be
// serialised, which the single-connection write pool guarantees.
func TestNoLockContentionUnderLoad(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.Write().Exec(
		`INSERT INTO accounts (email, provider, auth_kind, imap_host, imap_port, secret_ref, created_at)
		 VALUES ('a@example.com', 'generic', 'password', 'localhost', 993, 'ref', 0)`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := s.Write().Exec(
		`INSERT INTO folders (account_id, name, path) VALUES (1, 'INBOX', 'INBOX')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	const (
		readers  = 50
		duration = 3 * time.Second
	)

	var (
		wg       sync.WaitGroup
		writes   atomic.Int64
		busyHits atomic.Int64
		stop     = make(chan struct{})
	)

	recordIfBusy := func(err error) {
		if err == nil {
			return
		}
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "database is locked") ||
			strings.Contains(msg, "sqlite_busy") ||
			strings.Contains(msg, "database table is locked") {
			busyHits.Add(1)
			return
		}
		t.Errorf("unexpected error: %v", err)
	}

	// One writer.
	wg.Go(func() {
		for uid := int64(1); ; uid++ {
			select {
			case <-stop:
				return
			default:
			}
			_, err := s.Write().Exec(
				`INSERT INTO messages (account_id, folder_id, uid, subject, internal_date)
				 VALUES (1, 1, ?, ?, ?)`, uid, "subject", uid)
			recordIfBusy(err)
			if err == nil {
				writes.Add(1)
			}
		}
	})

	// Fifty readers, running the same query the message list issues.
	for range readers {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				rows, err := s.Read().Query(
					`SELECT id, subject FROM messages
					 WHERE folder_id = 1 ORDER BY internal_date DESC LIMIT 50`)
				if err != nil {
					recordIfBusy(err)
					continue
				}
				for rows.Next() {
					var id int64
					var subject string
					if err := rows.Scan(&id, &subject); err != nil {
						t.Errorf("scan: %v", err)
						break
					}
				}
				if err := rows.Err(); err != nil {
					recordIfBusy(err)
				}
				if err := rows.Close(); err != nil {
					t.Errorf("rows.Close(): %v", err)
				}
			}
		})
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()

	if got := busyHits.Load(); got != 0 {
		t.Errorf("observed %d lock-contention errors, want 0", got)
	}
	if writes.Load() == 0 {
		t.Fatal("writer made no progress; the test proves nothing")
	}
	t.Logf("completed %d writes against %d readers with no contention", writes.Load(), readers)

	// The row count must match exactly what the writer reported. A mismatch
	// would mean a write was acknowledged but lost, which is worse than
	// contention.
	var count int64
	if err := s.Read().QueryRow(`SELECT count(*) FROM messages`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != writes.Load() {
		t.Errorf("row count = %d, writer reported %d successful writes", count, writes.Load())
	}

	// Every inserted row also went through the FTS insert trigger. If the
	// trigger were to fail under load the index would silently drift.
	var indexed int64
	if err := s.Read().QueryRow(`SELECT count(*) FROM fts_messages`).Scan(&indexed); err != nil {
		t.Fatalf("count fts rows: %v", err)
	}
	if indexed != count {
		t.Errorf("FTS holds %d rows but messages holds %d; the trigger fell behind under load", indexed, count)
	}
}
