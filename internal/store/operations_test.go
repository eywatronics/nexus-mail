package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func queueOp(t *testing.T, s *Store, op model.Operation) int64 {
	t.Helper()

	id, err := s.EnqueueOperation(context.Background(), op)
	if err != nil {
		t.Fatalf("EnqueueOperation() error: %v", err)
	}
	return id
}

func claim(t *testing.T, s *Store, accountID int64) []model.Operation {
	t.Helper()

	ops, err := s.ClaimOperations(context.Background(), accountID, 50)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	return ops
}

// Everything the worker needs to send a command has to survive the trip
// through the database. A queue that loses the flags is a queue that marks the
// wrong thing read.
func TestOperationRoundTripsEveryField(t *testing.T) {
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, UIDValidity: 42,
		Kind:  model.OpAddFlags,
		UIDs:  []uint32{7, 9, 11},
		Flags: []string{model.FlagSeen, model.FlagFlagged},
	})

	ops := claim(t, s, acct)
	if len(ops) != 1 {
		t.Fatalf("claimed %d operations, want 1", len(ops))
	}

	got := ops[0]
	if got.Kind != model.OpAddFlags {
		t.Errorf("Kind = %q, want %q", got.Kind, model.OpAddFlags)
	}
	if got.UIDValidity != 42 {
		t.Errorf("UIDValidity = %d, want 42", got.UIDValidity)
	}
	if len(got.UIDs) != 3 || got.UIDs[0] != 7 || got.UIDs[2] != 11 {
		t.Errorf("UIDs = %v, want [7 9 11]", got.UIDs)
	}
	if len(got.Flags) != 2 {
		t.Errorf("Flags = %v, want two", got.Flags)
	}
	if got.State != model.OpPending {
		t.Errorf("State = %q, want pending", got.State)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero; the queue orders on it")
	}
}

func TestMoveOperationCarriesItsDestination(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertFolders(ctx, acct, []model.Folder{{Path: "Arşiv", Name: "Arşiv"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	archive := folderIDByPath(t, s, acct, "Arşiv")

	queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, UIDValidity: 1,
		Kind: model.OpMove, UIDs: []uint32{3}, TargetFolderID: archive,
	})

	ops := claim(t, s, acct)
	if len(ops) != 1 || ops[0].TargetFolderID != archive {
		t.Errorf("move operation lost its destination: %+v", ops)
	}
}

// The queue is drained oldest first. Two changes to the same message applied
// out of order leave it in the state the user asked for second-to-last.
func TestClaimReturnsOldestFirst(t *testing.T) {
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	for uid := uint32(1); uid <= 3; uid++ {
		queueOp(t, s, model.Operation{
			AccountID: acct, FolderID: inbox, UIDValidity: 1,
			Kind: model.OpAddFlags, UIDs: []uint32{uid}, Flags: []string{model.FlagSeen},
		})
	}

	ops := claim(t, s, acct)
	if len(ops) != 3 {
		t.Fatalf("claimed %d operations, want 3", len(ops))
	}
	for i, want := range []uint32{1, 2, 3} {
		if ops[i].UIDs[0] != want {
			t.Errorf("operation %d acts on UID %d, want %d", i, ops[i].UIDs[0], want)
		}
	}
}

// Two accounts share one database. Draining one must not send the other's
// changes down the wrong connection.
func TestClaimIsScopedToOneAccount(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	work, inbox := seedInbox(t, s)

	personal, err := s.InsertAccount(ctx, model.Account{
		Email: "personal@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "p", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}

	if err := s.UpsertFolders(ctx, personal, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	personalInbox := folderIDByPath(t, s, personal, "INBOX")

	queueOp(t, s, model.Operation{AccountID: work, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{1}})
	queueOp(t, s, model.Operation{AccountID: personal, FolderID: personalInbox, Kind: model.OpDelete, UIDs: []uint32{1}})

	if got := claim(t, s, work); len(got) != 1 || got[0].AccountID != work {
		t.Errorf("claimed %d operations for the work account, want only its own", len(got))
	}
}

func TestDoneOperationsAreNotClaimedAgain(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	id := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{1},
	})

	if err := s.MarkOperationDone(ctx, id); err != nil {
		t.Fatalf("MarkOperationDone() error: %v", err)
	}
	if got := claim(t, s, acct); len(got) != 0 {
		t.Errorf("a completed operation was claimed again: %+v", got)
	}
}

// A failure has to push the retry into the future, or the worker spins on the
// same broken operation as fast as the network allows.
func TestFailedOperationIsHeldBackUntilItsRetryTime(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	id := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{1},
	})

	if err := s.MarkOperationFailed(ctx, id, "connection reset", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("MarkOperationFailed() error: %v", err)
	}

	if got := claim(t, s, acct); len(got) != 0 {
		t.Errorf("an operation waiting for its retry time was claimed: %+v", got)
	}

	// Once the time passes it comes back, carrying what went wrong.
	if err := s.MarkOperationFailed(ctx, id, "connection reset", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("second MarkOperationFailed() error: %v", err)
	}
	got := claim(t, s, acct)
	if len(got) != 1 {
		t.Fatalf("claimed %d operations after the retry time passed, want 1", len(got))
	}
	if got[0].Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", got[0].Attempts)
	}
	if got[0].LastError == "" {
		t.Error("LastError is empty; the user is owed a reason")
	}
}

// This is the rule that keeps one bad change from freezing everything else.
// A queue that stopped at the first failure would leave a whole account stuck
// because one message was deleted on the server first.
func TestOneStuckOperationDoesNotBlockTheRest(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	stuck := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{1},
	})
	queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpAddFlags,
		UIDs: []uint32{2}, Flags: []string{model.FlagSeen},
	})

	if err := s.MarkOperationFailed(ctx, stuck, "no such message", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("MarkOperationFailed() error: %v", err)
	}

	got := claim(t, s, acct)
	if len(got) != 1 || got[0].UIDs[0] != 2 {
		t.Errorf("claimed %+v; the healthy operation should still flow", got)
	}
}

// A permanently failed operation stops being retried but stays on record, so
// the user can be told what did not happen.
func TestPermanentFailureLeavesARecord(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	id := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{1},
	})
	if err := s.MarkOperationPermanentlyFailed(ctx, id, "mailbox is read-only"); err != nil {
		t.Fatalf("MarkOperationPermanentlyFailed() error: %v", err)
	}

	if got := claim(t, s, acct); len(got) != 0 {
		t.Errorf("a permanently failed operation was claimed again: %+v", got)
	}

	var state, lastErr string
	if err := s.Read().QueryRowContext(ctx,
		`SELECT state, last_error FROM operations WHERE id = ?`, id).Scan(&state, &lastErr); err != nil {
		t.Fatalf("reading the operation: %v", err)
	}
	if state != string(model.OpFailed) {
		t.Errorf("state = %q, want failed", state)
	}
	if lastErr == "" {
		t.Error("last_error is empty; there is nothing to tell the user")
	}
}

// The UIDVALIDITY stamp is the whole point of storing it. An operation queued
// against a mailbox the server has since recreated names UIDs that now belong
// to different messages, and applying it would delete the wrong mail.
func TestDroppedOperationsAreRecordedNotDiscarded(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	id := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, UIDValidity: 1,
		Kind: model.OpDelete, UIDs: []uint32{55},
	})

	if err := s.MarkOperationDropped(ctx, id); err != nil {
		t.Fatalf("MarkOperationDropped() error: %v", err)
	}
	if got := claim(t, s, acct); len(got) != 0 {
		t.Errorf("a dropped operation was claimed: %+v", got)
	}

	// Silence would be data loss the user never hears about; the record is
	// what lets the UI say "N pending changes could not be applied".
	dropped, err := s.CountOperations(ctx, acct, model.OpDropped)
	if err != nil {
		t.Fatalf("CountOperations() error: %v", err)
	}
	if dropped != 1 {
		t.Errorf("CountOperations(dropped) = %d, want 1", dropped)
	}
}

func TestClaimHonoursItsLimit(t *testing.T) {
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	for uid := uint32(1); uid <= 10; uid++ {
		queueOp(t, s, model.Operation{
			AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{uid},
		})
	}

	ops, err := s.ClaimOperations(context.Background(), acct, 4)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	if len(ops) != 4 {
		t.Errorf("claimed %d operations, want the 4 asked for", len(ops))
	}
}

// Removing an account must not leave its queued changes behind to be sent
// down somebody else's connection later.
func TestRemovingAnAccountClearsItsQueue(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{1},
	})
	if _, err := s.Write().ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, acct); err != nil {
		t.Fatalf("deleting the account: %v", err)
	}

	var left int
	if err := s.Read().QueryRowContext(ctx, `SELECT count(*) FROM operations`).Scan(&left); err != nil {
		t.Fatalf("counting operations: %v", err)
	}
	if left != 0 {
		t.Errorf("%d operations outlived their account", left)
	}
}

func TestEnqueueRejectsAnOperationWithNoUIDs(t *testing.T) {
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	// An empty operation would send a command naming nothing, which some
	// servers answer with an error and others with a surprise.
	if _, err := s.EnqueueOperation(context.Background(), model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete,
	}); err == nil {
		t.Error("EnqueueOperation() accepted an operation with no UIDs")
	}

	// A UID only means something inside one folder. Without one the worker
	// would not know which mailbox to select.
	if _, err := s.EnqueueOperation(context.Background(), model.Operation{
		AccountID: acct, Kind: model.OpDelete, UIDs: []uint32{1},
	}); err == nil {
		t.Error("EnqueueOperation() accepted an operation with no folder")
	}
}

// The banner that tells the user about dropped changes needs a way to go
// away, and the only honest way is to stop the count being true.
func TestForgettingFinishedOperationsClearsTheRecord(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	dropped := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{1},
	})
	failed := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{2},
	})
	pending := queueOp(t, s, model.Operation{
		AccountID: acct, FolderID: inbox, Kind: model.OpDelete, UIDs: []uint32{3},
	})
	if err := s.MarkOperationDropped(ctx, dropped); err != nil {
		t.Fatalf("MarkOperationDropped() error: %v", err)
	}
	if err := s.MarkOperationPermanentlyFailed(ctx, failed, "read-only"); err != nil {
		t.Fatalf("MarkOperationPermanentlyFailed() error: %v", err)
	}

	if err := s.ForgetFinishedOperations(ctx, acct); err != nil {
		t.Fatalf("ForgetFinishedOperations() error: %v", err)
	}

	for _, state := range []model.OperationState{model.OpDropped, model.OpFailed} {
		n, err := s.CountOperations(ctx, acct, state)
		if err != nil {
			t.Fatalf("CountOperations(%s) error: %v", state, err)
		}
		if n != 0 {
			t.Errorf("CountOperations(%s) = %d, want 0", state, n)
		}
	}

	// Work still on its way must survive: acknowledging a failure is not the
	// same as cancelling everything else the user asked for.
	var left int
	if err := s.Read().QueryRowContext(ctx,
		`SELECT count(*) FROM operations WHERE id = ?`, pending).Scan(&left); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if left != 1 {
		t.Error("acknowledging failures also threw away pending work")
	}
}
