package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexusmail/internal/model"
	"nexusmail/internal/store"
)

// waitUntil polls a condition briefly. The watch loop runs in its own
// goroutine, so its effects are not visible the instant it starts.
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never became true")
}

func enqueue(t *testing.T, s *store.Store, op model.Operation) int64 {
	t.Helper()

	id, err := s.EnqueueOperation(context.Background(), op)
	if err != nil {
		t.Fatalf("EnqueueOperation() error: %v", err)
	}
	return id
}

func opState(t *testing.T, s *store.Store, id int64) model.OperationState {
	t.Helper()

	var state string
	if err := s.Read().QueryRow(`SELECT state FROM operations WHERE id = ?`, id).Scan(&state); err != nil {
		t.Fatalf("reading operation %d: %v", id, err)
	}
	return model.OperationState(state)
}

func TestDrainSendsAFlagChangeToTheServer(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2)

	id := enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpAddFlags, UIDs: []uint32{1}, Flags: []string{model.FlagSeen},
	})

	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}

	writes := be.recordedWrites()
	if len(writes) != 1 || writes[0].kind != "store" || !writes[0].add {
		t.Fatalf("the server received %+v, want one flag add", writes)
	}
	if writes[0].path != "INBOX" {
		t.Errorf("the command went to %q, want INBOX", writes[0].path)
	}
	if got := opState(t, s, id); got != model.OpDone {
		t.Errorf("state = %q, want done", got)
	}
}

func TestDrainSendsAMoveAndADelete(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	be.addFolder("Arşiv", nil, 200)

	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)
	archive := folderByPath(t, s, acct.ID, "Arşiv")

	enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpMove, UIDs: []uint32{1}, TargetFolderID: archive.ID,
	})
	enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpDelete, UIDs: []uint32{2},
	})

	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}

	writes := be.recordedWrites()
	if len(writes) != 2 {
		t.Fatalf("the server received %d commands, want 2: %+v", len(writes), writes)
	}
	if writes[0].kind != "move" || writes[0].dest != "Arşiv" {
		t.Errorf("first command = %+v, want a move to Arşiv", writes[0])
	}
	if writes[1].kind != "expunge" {
		t.Errorf("second command = %+v, want an expunge", writes[1])
	}
}

// This is the one that matters most. The user queued a delete while offline;
// by the time the connection came back the server had recreated the mailbox,
// so UID 55 is now a different message. Sending the command would delete
// somebody's mail and nobody would ever know why.
func TestDrainDropsOperationsQueuedAgainstAnOldMailbox(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2)

	id := enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID,
		UIDValidity: folder.UIDValidity, // the generation it was queued against
		Kind:        model.OpDelete, UIDs: []uint32{55},
	})

	// The server recreates the mailbox: same name, brand new UID space.
	be.setUIDValidity("INBOX", 999)

	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}

	if writes := be.recordedWrites(); len(writes) != 0 {
		t.Errorf("the server was sent %+v against a recreated mailbox", writes)
	}
	if got := opState(t, s, id); got != model.OpDropped {
		t.Errorf("state = %q, want dropped", got)
	}
}

// Dropped work must stay visible. Data loss the user is told about is a
// different thing from data loss they are not.
func TestDroppedOperationsAreCountable(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1)

	enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpDelete, UIDs: []uint32{1},
	})
	be.setUIDValidity("INBOX", 999)

	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}

	n, err := s.CountOperations(ctx, acct.ID, model.OpDropped)
	if err != nil {
		t.Fatalf("CountOperations() error: %v", err)
	}
	if n != 1 {
		t.Errorf("CountOperations(dropped) = %d, want 1", n)
	}
}

// A dropped connection is worth trying again. Giving up on it would lose a
// change the user made, for a reason that fixes itself.
func TestDrainRetriesATransientFailure(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1)

	id := enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpAddFlags, UIDs: []uint32{1}, Flags: []string{model.FlagSeen},
	})
	be.writeErr = errors.New("connection reset by peer")

	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}

	if got := opState(t, s, id); got != model.OpPending {
		t.Errorf("state = %q, want it still pending for a retry", got)
	}
}

// A server that says "this mailbox is read-only" will say it again tomorrow.
// Retrying forever would keep an unfixable change in the queue and keep
// telling the user something is in flight.
func TestDrainStopsRetryingAPermanentFailure(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1)

	id := enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpAddFlags, UIDs: []uint32{1}, Flags: []string{model.FlagSeen},
	})
	be.writeErr = errors.New("STORE failed: mailbox is read-only")

	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}

	if got := opState(t, s, id); got != model.OpFailed {
		t.Errorf("state = %q, want failed", got)
	}
}

// One unusable change must not hold back the others. Otherwise a single
// message deleted on the server freezes every other change on the account.
func TestOneBadOperationDoesNotStopTheDrain(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2)

	// Queued against a generation that no longer matches: this one is dropped.
	bad := enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: 4242,
		Kind: model.OpDelete, UIDs: []uint32{1},
	})
	good := enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpAddFlags, UIDs: []uint32{2}, Flags: []string{model.FlagSeen},
	})

	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}

	if got := opState(t, s, bad); got != model.OpDropped {
		t.Errorf("the bad operation is %q, want dropped", got)
	}
	if got := opState(t, s, good); got != model.OpDone {
		t.Errorf("the good operation is %q, want done — it was blocked", got)
	}
}

// Draining twice must not send the same command twice. The loop calls this on
// every pass.
func TestDrainDoesNotResendCompletedWork(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1)

	enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpAddFlags, UIDs: []uint32{1}, Flags: []string{model.FlagSeen},
	})

	for range 3 {
		if err := eng.DrainQueue(ctx, acct); err != nil {
			t.Fatalf("DrainQueue() error: %v", err)
		}
	}

	if writes := be.recordedWrites(); len(writes) != 1 {
		t.Errorf("the server received %d commands after three drains, want 1", len(writes))
	}
}

// An empty queue is the normal state, and it must not cost a connection.
func TestDrainOnAnEmptyQueueDoesNothing(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, _, acct, _ := syncedInbox(t, be, 1)

	before := be.selectCalls
	if err := eng.DrainQueue(ctx, acct); err != nil {
		t.Fatalf("DrainQueue() error: %v", err)
	}
	if be.selectCalls != before {
		t.Errorf("an empty drain selected a mailbox")
	}
	if writes := be.recordedWrites(); len(writes) != 0 {
		t.Errorf("an empty drain sent %+v", writes)
	}
}

// The watch loop is what makes the queue drain without the user doing
// anything. A queue nothing drains is a queue that never reaches the server.
func TestWatchDrainsTheQueue(t *testing.T) {
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1)

	enqueue(t, s, model.Operation{
		AccountID: acct.ID, FolderID: folder.ID, UIDValidity: folder.UIDValidity,
		Kind: model.OpAddFlags, UIDs: []uint32{1}, Flags: []string{model.FlagSeen},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eng.Watch(ctx, acct, nil) }()

	waitUntil(t, func() bool {
		for _, w := range be.recordedWrites() {
			if w.kind == "store" {
				return true
			}
		}
		return false
	})
	cancel()
	<-done
}
