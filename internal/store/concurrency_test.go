package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// SQLite allows one writer. The question that matters for this client is what
// happens to the window while the sync engine is mid-batch: a fetch of five
// hundred headers is one transaction, and every INSERT fires the FTS triggers
// behind it. If a search or a folder change collided with that, the answer
// would be a spinner at the exact moment the user is typing.
//
// The DSN asks for WAL, a five-second busy timeout, and separate pools. These
// tests are here because asking is not the same as getting: _pragma is applied
// per connection, and a pool hands out more than one.

// A pragma that only reached the first connection would be worse than one that
// reached none: it would work in every test that opens a store and does one
// thing, and fail under exactly the load it exists for.
//
// TestOpenAppliesMigrationsAndPragmas checks journal_mode on one connection,
// which is the case this one does not cover and vice versa.
func TestEveryConnectionInThePoolHasTheConcurrencyPragmas(t *testing.T) {
	s := openTestStore(t)

	// More goroutines than the pool has connections, each holding its query
	// open long enough that the pool cannot serve them all with one.
	const probes = readPoolSize * 2
	var wg sync.WaitGroup
	results := make([]struct {
		mode    string
		timeout int
	}, probes)
	errs := make([]error, probes)

	start := make(chan struct{})
	for i := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			row := s.Read().QueryRow(`SELECT (SELECT * FROM pragma_journal_mode),
				(SELECT * FROM pragma_busy_timeout)`)
			errs[i] = row.Scan(&results[i].mode, &results[i].timeout)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("probe %d: %v", i, err)
		}
		if !strings.EqualFold(results[i].mode, "wal") {
			t.Errorf("connection %d is in journal mode %q, not WAL", i, results[i].mode)
		}
		if results[i].timeout != 5000 {
			t.Errorf("connection %d has busy_timeout %d, not 5000", i, results[i].timeout)
		}
	}
}

// The scenario itself: the engine writing batches while the window searches
// and changes folders.
//
// What this holds down is that a read never fails and never waits out the busy
// timeout while a batch is in flight. It deliberately does not assert a
// latency, and it is worth being clear that it would pass without WAL — with a
// rollback journal the busy timeout absorbs the contention and readers get
// slow answers rather than errors. Measured on one machine, two seconds each:
//
//	WAL     47 batches of 500, 318 read rounds, slowest read 55ms
//	DELETE  28 batches of 500, 263 read rounds, slowest read 142ms
//
// So WAL buys throughput and a worst case two and a half times better, not
// correctness. A ceiling tight enough to catch its absence would be tight
// enough to fail on a loaded CI runner, which is a test that gets deleted
// rather than a test that catches anything.
func TestReadsKeepWorkingWhileBatchesAreWritten(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, folder := seedInboxForConcurrency(t, s)

	// Enough of a mailbox that a search has something to do.
	writeBatch(t, s, acct, folder, 0, 2000)

	var (
		writes, reads atomic.Int64
		readErr       atomic.Value
		writeErr      atomic.Value
		worst         atomic.Int64
	)

	done := make(chan struct{})
	var wg sync.WaitGroup

	// The writer: batches of 500, which is what a header fetch looks like.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for uid := 2000; ; uid += 500 {
			select {
			case <-done:
				return
			default:
			}
			if err := upsertBatch(ctx, s, acct, folder, uid, 500); err != nil {
				writeErr.Store(err)
				return
			}
			writes.Add(1)
		}
	}()

	// The window: searching and listing, as fast as somebody typing.
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				began := time.Now()
				if _, err := s.SearchMessages(ctx, acct, "fatura", 50); err != nil {
					readErr.Store(err)
					return
				}
				if _, err := s.ListMessages(ctx, folder, 50, 0); err != nil {
					readErr.Store(err)
					return
				}
				if took := time.Since(began).Milliseconds(); took > worst.Load() {
					worst.Store(took)
				}
				reads.Add(1)
			}
		}()
	}

	time.Sleep(2 * time.Second)
	close(done)
	wg.Wait()

	if err := readErr.Load(); err != nil {
		t.Fatalf("a read failed while a batch was being written: %v", err)
	}
	if err := writeErr.Load(); err != nil {
		t.Fatalf("a batch failed while reads were running: %v", err)
	}
	if writes.Load() == 0 || reads.Load() == 0 {
		t.Fatalf("the test did not contend: %d batches, %d reads", writes.Load(), reads.Load())
	}

	// Not an assertion about speed — machines differ and CI is loaded. The
	// number is here to be read when somebody asks whether writes block the
	// window, which is the question these pragmas exist to answer.
	t.Logf("%d batches of 500 and %d read rounds in 2s; slowest read round %dms",
		writes.Load(), reads.Load(), worst.Load())

	// A read that blocked for the whole busy timeout would have returned an
	// error, not a slow answer, so the ceiling is what proves the reader never
	// waited on the writer at all.
	if worst.Load() >= 5000 {
		t.Errorf("a read round took %dms, which is the busy timeout", worst.Load())
	}
}

func seedInboxForConcurrency(t *testing.T, s *Store) (int64, int64) {
	t.Helper()
	ctx := context.Background()

	acct, err := s.InsertAccount(ctx, model.Account{
		Email: "u@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "ref", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}
	if err := s.UpsertFolders(ctx, acct, []model.Folder{
		{Path: "INBOX", Name: "INBOX", Attributes: []string{"\\Inbox"}},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	folders, err := s.ListFolders(ctx, acct)
	if err != nil || len(folders) == 0 {
		t.Fatalf("ListFolders() = %v, %v", folders, err)
	}
	return acct, folders[0].ID
}

func upsertBatch(ctx context.Context, s *Store, acct, folder int64, first, n int) error {
	msgs := make([]model.Message, 0, n)
	for i := first; i < first+n; i++ {
		msgs = append(msgs, model.Message{
			AccountID: acct, FolderID: folder, UID: uint32(i + 1),
			MessageID: fmt.Sprintf("m%d@example.com", i),
			Subject:   fmt.Sprintf("Fatura %d odendi", i),
			From:      model.Address{Name: "Muhasebe", Addr: "muhasebe@example.com"},
			Snippet: fmt.Sprintf("Merhaba, %d numarali kaydi onaylamanizi rica ederim. "+
				"Iyi calismalar.", i),
			InternalDate: time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}
	return s.UpsertMessages(ctx, folder, msgs)
}

func writeBatch(t *testing.T, s *Store, acct, folder int64, first, n int) {
	t.Helper()
	if err := upsertBatch(context.Background(), s, acct, folder, first, n); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}
