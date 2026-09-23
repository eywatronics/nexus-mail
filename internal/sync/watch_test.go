package sync

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/store"
)

// The backoff schedule is what stops a client from hammering a server that
// just went down, and the cap is what stops it from giving up for an hour.
// Both are asserted as a range because of the jitter, which is the point of
// the jitter.
func TestBackoffGrowsAndIsCapped(t *testing.T) {
	var previous time.Duration

	for attempt := range 20 {
		d := backoff(attempt)

		if d < 0 {
			t.Fatalf("backoff(%d) = %s; a negative wait would spin", attempt, d)
		}
		if d > backoffCap {
			t.Errorf("backoff(%d) = %s, above the %s cap", attempt, d, backoffCap)
		}
		// Growth is only expected until the cap; after it jitter dominates.
		if attempt > 0 && attempt < 6 && d <= previous/2 {
			t.Errorf("backoff(%d) = %s did not grow past backoff(%d) = %s",
				attempt, d, attempt-1, previous)
		}
		previous = d
	}
}

// Jitter exists so that a thousand clients that lost the same server do not
// all come back at the same instant. Identical values would defeat it.
func TestBackoffIsJittered(t *testing.T) {
	seen := map[time.Duration]bool{}
	for range 30 {
		seen[backoff(4)] = true
	}
	if len(seen) < 5 {
		t.Errorf("backoff(4) produced %d distinct values in 30 tries; the jitter is not working",
			len(seen))
	}
}

// The first thing a connection is good for is finding out what was missed
// while there was no connection.
func TestWatchSyncsOnConnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		eng, s, acct, folder := syncedInbox(t, be, 1, 2)

		be.addMessages("INBOX", 3)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		synctest.Wait()
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Watch() error: %v", err)
		}

		if got := storedUIDs(t, s, folder.ID); len(got) != 3 {
			t.Errorf("folder holds %v after the connect sync, want three messages", got)
		}
	})
}

// This is what IDLE is for: the server speaks, and the mail appears without
// anybody polling.
func TestWatchSyncsWhenIdleWakes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		eng, s, acct, folder := syncedInbox(t, be, 1)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		// Let the connect-time sync finish and the loop settle into IDLE.
		synctest.Wait()

		be.addMessages("INBOX", 2)
		be.wakeIdle()
		synctest.Wait()

		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Watch() error: %v", err)
		}

		if got := storedUIDs(t, s, folder.ID); len(got) != 2 {
			t.Errorf("folder holds %v after the wake, want two messages", got)
		}
	})
}

// Cancelling has to actually stop the loop, and stop it cleanly — a
// supervisor that returned the context error would make every shutdown look
// like a failure in the logs.
func TestWatchStopsCleanlyOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		eng, _, acct, _ := syncedInbox(t, be, 1)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		synctest.Wait()
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Watch() error = %v, want nil on cancellation", err)
			}
		case <-time.After(time.Minute):
			t.Fatal("Watch() did not return after cancellation")
		}
	})
}

// A dropped connection must not become a spin: the loop waits, reconnects,
// and — this is the part that matters — runs a delta pass again, because
// anything that changed while it was disconnected was never announced.
func TestWatchReconnectsAndResyncsAfterADrop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		eng, s, acct, folder := syncedInbox(t, be, 1)

		be.idleErr = errors.New("connection reset by peer")

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		synctest.Wait()

		// While it is disconnected and waiting, the mailbox moves on. Nothing
		// will tell the client about this; only the post-reconnect sync will.
		be.addMessages("INBOX", 2)
		be.mu.Lock()
		be.idleErr = nil
		be.mu.Unlock()

		// Past the first backoff step so the reconnect happens.
		time.Sleep(2 * backoffCap)
		synctest.Wait()

		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Watch() error: %v", err)
		}

		if got := storedUIDs(t, s, folder.ID); len(got) != 2 {
			t.Errorf("folder holds %v after the reconnect, want the message that "+
				"arrived while it was down", got)
		}
	})
}

// A server that refuses every connection must not become a busy loop. The
// assertion is on elapsed fake time: without a wait, many attempts would take
// no time at all.
func TestWatchWaitsBetweenFailedReconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		_, s, acct := newTestEngine(t, be)

		dialErr := errors.New("dial: connection refused")
		eng := New(s, func(context.Context, int64) (imapx.MailBackend, error) {
			return nil, dialErr
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		start := time.Now()
		time.Sleep(30 * time.Second)
		synctest.Wait()
		cancel()
		<-done

		if elapsed := time.Since(start); elapsed < 30*time.Second {
			t.Errorf("only %s of fake time passed; the loop is not waiting", elapsed)
		}
	})
}

// A folder the initial sync left empty is not watched into existence — the
// supervisor only refreshes what has been opened. Syncing every folder on
// every wake would turn one new message into a scan of the whole account.
func TestWatchOnlyRefreshesFoldersThatHaveBeenSynced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		be.addFolder("INBOX", []string{"\\Inbox"}, 100)
		be.addFolder("Arşiv", nil, 200)
		be.addMessages("INBOX", 1)
		be.addMessages("Arşiv", 1, 2, 3)

		eng, s, acct := newTestEngine(t, be)
		if err := eng.InitialSync(context.Background(), acct); err != nil {
			t.Fatalf("InitialSync() error: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		synctest.Wait()
		cancel()
		<-done

		archive := folderByPath(t, s, acct.ID, "Arşiv")
		if got := storedUIDs(t, s, archive.ID); len(got) != 0 {
			t.Errorf("the untouched folder was filled in behind the user's back: %v", got)
		}
	})
}

// Watch must not be reachable for an account with no inbox; there would be
// nothing to IDLE on and the loop would spin.
func TestWatchReportsAnAccountWithNoInbox(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("Arşiv", nil, 200)

	eng, s, acct := newTestEngine(t, be)
	if err := eng.InitialSync(context.Background(), acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}
	_ = s

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := eng.Watch(ctx, model.Account{ID: acct.ID}, nil); err == nil {
		t.Error("Watch() succeeded for an account with no inbox")
	}
}

// The retention window only matters once live sync is running: the initial
// fetch is capped, but nothing caps growth afterwards. So the purge belongs
// here, on the loop that does the growing.
func TestWatchPurgesOutsideTheRetentionWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

		// Age two of them past the window, behind the engine's back — the
		// server still has them, which is the point: the purge is local.
		ageStoredMessage(t, s, folder.ID, 1, 400*24*time.Hour)
		ageStoredMessage(t, s, folder.ID, 2, 400*24*time.Hour)

		eng.SetRetention(model.RetentionPolicy{MaxAge: 365 * 24 * time.Hour})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		synctest.Wait()
		cancel()
		<-done

		left := storedUIDs(t, s, folder.ID)
		if len(left) != 1 || left[0] != 3 {
			t.Errorf("folder holds %v after the purge, want only the recent message", left)
		}
	})
}

// "Keep everything" has to actually keep everything, including on the loop
// that would otherwise be trimming in the background.
func TestWatchPurgesNothingWhenTheWindowIsOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		eng, s, acct, folder := syncedInbox(t, be, 1, 2)

		ageStoredMessage(t, s, folder.ID, 1, 4000*24*time.Hour)
		eng.SetRetention(model.RetentionPolicy{})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		synctest.Wait()
		cancel()
		<-done

		if left := storedUIDs(t, s, folder.ID); len(left) != 2 {
			t.Errorf("folder holds %v, want both messages kept", left)
		}
	})
}

// The exemption has to survive the trip through the loop, not just the store.
// Ageing out a message somebody starred is the one failure of this feature
// that would be unforgivable.
func TestWatchNeverPurgesAFlaggedMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		be := newFakeBackend()
		eng, s, acct, folder := syncedInbox(t, be, 1, 2)

		ageStoredMessage(t, s, folder.ID, 1, 4000*24*time.Hour)
		ageStoredMessage(t, s, folder.ID, 2, 4000*24*time.Hour)
		if err := s.SetMessageFlags(context.Background(), folder.ID,
			[]model.FlagUpdate{{UID: 1, Flags: []string{model.FlagFlagged}}}); err != nil {
			t.Fatalf("SetMessageFlags() error: %v", err)
		}

		eng.SetRetention(model.RetentionPolicy{MaxAge: 365 * 24 * time.Hour})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- eng.Watch(ctx, acct, nil) }()

		synctest.Wait()
		cancel()
		<-done

		left := storedUIDs(t, s, folder.ID)
		if len(left) != 1 || left[0] != 1 {
			t.Errorf("folder holds %v, want the flagged message kept and the other gone", left)
		}
	})
}

// ageStoredMessage backdates a stored message without going through the
// engine, standing in for mail that has simply been sitting there for a year.
func ageStoredMessage(t *testing.T, s *store.Store, folderID int64, uid uint32, age time.Duration) {
	t.Helper()

	if _, err := s.Write().Exec(
		`UPDATE messages SET internal_date = ? WHERE folder_id = ? AND uid = ?`,
		time.Now().Add(-age).Unix(), folderID, uid); err != nil {
		t.Fatalf("ageing UID %d: %v", uid, err)
	}
}
