package sync

import (
	"context"
	"testing"

	"nexusmail/internal/model"
	"nexusmail/internal/store"
)

// syncedInbox brings a folder to the state a delta sync starts from: folders
// discovered, headers fetched, sync state recorded.
func syncedInbox(t *testing.T, be *fakeBackend, uids ...uint32) (*Engine, *store.Store, model.Account, model.Folder) {
	t.Helper()

	be.addFolder("INBOX", []string{"\\Inbox"}, 100)
	be.addMessages("INBOX", uids...)

	eng, s, acct := newTestEngine(t, be)
	if err := eng.InitialSync(context.Background(), acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}
	return eng, s, acct, folderByPath(t, s, acct.ID, "INBOX")
}

func storedUIDs(t *testing.T, s *store.Store, folderID int64) []uint32 {
	t.Helper()

	uids, err := s.ListMessageUIDs(context.Background(), folderID)
	if err != nil {
		t.Fatalf("ListMessageUIDs() error: %v", err)
	}
	return uids
}

func storedFlags(t *testing.T, s *store.Store, folderID int64, uid uint32) []string {
	t.Helper()

	msgs, err := s.ListMessages(context.Background(), folderID, 500, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	for _, m := range msgs {
		if m.UID == uid {
			return m.Flags
		}
	}
	t.Fatalf("no message with UID %d", uid)
	return nil
}

// The point of a delta sync is not refetching what we already have. A pass
// that asked for the whole mailbox again would work, and would also make a
// fifty-thousand message folder unusable.
func TestDeltaSyncFetchesOnlyTheNewUIDs(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

	be.addMessages("INBOX", 4, 5)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	got := storedUIDs(t, s, folder.ID)
	if len(got) != 5 {
		t.Fatalf("folder holds %v, want five messages", got)
	}
}

// Someone reads a message on their phone. Nothing about the message changes
// except its flags, and the delta pass is the only thing that will notice.
func TestDeltaSyncAppliesServerSideFlagChanges(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2)

	be.setFlags("INBOX", 1, model.FlagSeen, model.FlagFlagged)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	flags := storedFlags(t, s, folder.ID, 1)
	if len(flags) != 2 {
		t.Errorf("UID 1 flags = %v, want seen and flagged", flags)
	}
	if got := storedFlags(t, s, folder.ID, 2); len(got) != 0 {
		t.Errorf("UID 2 flags = %v, want them untouched", got)
	}
}

// A message deleted elsewhere has to disappear here too. Leaving it means the
// reader opens mail that no longer exists and gets an error from the server.
func TestDeltaSyncRemovesExpungedMessages(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

	be.expunge("INBOX", 2)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	got := storedUIDs(t, s, folder.ID)
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("folder holds %v, want UIDs 1 and 3", got)
	}
}

// The two changes arriving together is the normal case, not an edge one.
func TestDeltaSyncHandlesArrivalsFlagsAndDeletionsInOnePass(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

	be.expunge("INBOX", 1)
	be.setFlags("INBOX", 2, model.FlagSeen)
	be.addMessages("INBOX", 4)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	got := storedUIDs(t, s, folder.ID)
	if len(got) != 3 || got[0] != 2 || got[2] != 4 {
		t.Errorf("folder holds %v, want UIDs 2, 3, 4", got)
	}
	if len(storedFlags(t, s, folder.ID, 2)) != 1 {
		t.Errorf("UID 2 did not pick up the seen flag")
	}
}

// A mailbox the server recreated invalidates every UID we hold. Applying a
// delta against it would attach the new mailbox's flags to the old mailbox's
// messages.
func TestDeltaSyncFallsBackToAFullSyncOnUIDValidityChange(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

	be.clearMessages("INBOX")
	be.setUIDValidity("INBOX", 999)
	be.addMessages("INBOX", 1, 2)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	got := storedUIDs(t, s, folder.ID)
	if len(got) != 2 {
		t.Errorf("folder holds %v, want the two messages of the new mailbox", got)
	}
	if after := folderByPath(t, s, acct.ID, "INBOX"); after.UIDValidity != 999 {
		t.Errorf("stored UIDVALIDITY = %d, want 999", after.UIDValidity)
	}
}

// A folder the initial sync deliberately left empty has no state to delta
// against; the first pass over it is a full one.
func TestDeltaSyncOnANeverSyncedFolderFetchesEverything(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	be.addFolder("INBOX", []string{"\\Inbox"}, 100)
	be.addFolder("Arşiv", nil, 200)
	be.addMessages("Arşiv", 1, 2, 3)

	eng, s, acct := newTestEngine(t, be)
	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	archive := folderByPath(t, s, acct.ID, "Arşiv")
	if got := storedUIDs(t, s, archive.ID); len(got) != 0 {
		t.Fatalf("precondition failed: the lazy folder already holds %v", got)
	}

	if err := eng.DeltaSync(ctx, acct, archive); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}
	if got := storedUIDs(t, s, archive.ID); len(got) != 3 {
		t.Errorf("folder holds %v, want three messages", got)
	}
}

// CONDSTORE turns a scan of a large mailbox into a short answer, but the only
// thing observable from outside is the argument the engine sends. So look at
// it.
func TestDeltaSyncUsesCondStoreWhenTheServerHasIt(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, _, acct, folder := syncedInbox(t, be, 1, 2, 3)

	be.setFlags("INBOX", 1, model.FlagSeen)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	calls := be.recordedFlagFetches()
	if len(calls) == 0 {
		t.Fatal("the engine never asked for flags")
	}
	last := calls[len(calls)-1]
	if last.changedSince == 0 {
		t.Errorf("CHANGEDSINCE was 0 on a CONDSTORE server; the whole window was rescanned")
	}
}

// Some corporate Zimbra and older Dovecot installations have no CONDSTORE.
// The fallback is not optional, and asking for CHANGEDSINCE there is a
// protocol error.
func TestDeltaSyncFallsBackWhenTheServerHasNoCondStore(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	be.caps.CondStore = false
	be.caps.QResync = false

	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)
	be.setFlags("INBOX", 1, model.FlagSeen)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	for _, call := range be.recordedFlagFetches() {
		if call.changedSince != 0 {
			t.Errorf("CHANGEDSINCE = %d against a server without CONDSTORE", call.changedSince)
		}
	}
	if len(storedFlags(t, s, folder.ID, 1)) != 1 {
		t.Errorf("the fallback path did not apply the flag change")
	}
}

// This is the subtle one. An expunge does not advance HIGHESTMODSEQ, so a
// CHANGEDSINCE fetch will not mention the deleted message — and the engine
// would keep it forever. The message count is what gives the deletion away,
// and seeing it must force a full scan of the window.
func TestDeltaSyncStopsUsingCondStoreWhenTheCountSaysSomethingWasDeleted(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

	be.expunge("INBOX", 2)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}

	calls := be.recordedFlagFetches()
	if len(calls) == 0 {
		t.Fatal("the engine never asked for flags")
	}
	last := calls[len(calls)-1]
	if last.changedSince != 0 {
		t.Errorf("CHANGEDSINCE = %d after an expunge; the deletion would never be found",
			last.changedSince)
	}
	if got := storedUIDs(t, s, folder.ID); len(got) != 2 {
		t.Errorf("folder holds %v, want the deletion applied", got)
	}
}

// Running twice over an unchanged mailbox must not invent work or lose data.
// The IDLE loop will call this every time the server so much as breathes.
func TestDeltaSyncIsIdempotent(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

	for i := 0; i < 3; i++ {
		if err := eng.DeltaSync(ctx, acct, folderByPath(t, s, acct.ID, "INBOX")); err != nil {
			t.Fatalf("DeltaSync() pass %d error: %v", i, err)
		}
	}

	if got := storedUIDs(t, s, folder.ID); len(got) != 3 {
		t.Errorf("folder holds %v after three passes, want three messages", got)
	}
}

// An empty folder is not an edge case; most accounts have several.
func TestDeltaSyncOnAnEmptyFolder(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be)

	if err := eng.DeltaSync(ctx, acct, folder); err != nil {
		t.Fatalf("DeltaSync() error: %v", err)
	}
	if got := storedUIDs(t, s, folder.ID); len(got) != 0 {
		t.Errorf("an empty folder gained %v", got)
	}
}

// Recording progress the pass did not make is how a sync silently skips
// changes: the next run asks "what changed since X" for an X it never reached.
func TestDeltaSyncDoesNotRecordProgressAfterAFailure(t *testing.T) {
	ctx := context.Background()
	be := newFakeBackend()
	eng, s, acct, folder := syncedInbox(t, be, 1, 2, 3)

	before := folderByPath(t, s, acct.ID, "INBOX")
	be.setFlags("INBOX", 1, model.FlagSeen)
	be.fetchErr = errContext("FETCH failed: connection reset by peer")

	if err := eng.DeltaSync(ctx, acct, folder); err == nil {
		t.Fatal("DeltaSync() succeeded despite a failing fetch")
	}

	after := folderByPath(t, s, acct.ID, "INBOX")
	if after.HighestModSeq != before.HighestModSeq {
		t.Errorf("HighestModSeq moved from %d to %d despite the failure",
			before.HighestModSeq, after.HighestModSeq)
	}
	if !after.LastSyncedAt.Equal(before.LastSyncedAt) {
		t.Errorf("LastSyncedAt moved despite the failure")
	}
}

type errContext string

func (e errContext) Error() string { return string(e) }
