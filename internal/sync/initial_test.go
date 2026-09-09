package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/store"
)

func newTestEngine(t *testing.T, be imapx.MailBackend) (*Engine, *store.Store, model.Account) {
	t.Helper()

	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open() error: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})

	acctID, err := s.InsertAccount(context.Background(), model.Account{
		Email: "user@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "ref", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}

	eng := New(s, func(context.Context, int64) (imapx.MailBackend, error) {
		return be, nil
	})
	return eng, s, model.Account{ID: acctID, Email: "user@example.com"}
}

func folderByPath(t *testing.T, s *store.Store, accountID int64, path string) model.Folder {
	t.Helper()

	folders, err := s.ListFolders(context.Background(), accountID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	for _, f := range folders {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no folder with path %q", path)
	return model.Folder{}
}

func TestInitialSyncStoresFoldersAndEagerHeaders(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addFolder("Sent", []string{"\\Sent"}, 200)
	be.addFolder("Projects", nil, 300)
	be.addMessages("INBOX", 1, 2, 3)
	be.addMessages("Sent", 10, 11)
	be.addMessages("Projects", 50, 51)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	folders, err := s.ListFolders(ctx, acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("stored %d folders, want 3", len(folders))
	}

	inbox, err := s.ListMessages(ctx, folderByPath(t, s, acct.ID, "INBOX").ID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(INBOX) error: %v", err)
	}
	if len(inbox) != 3 {
		t.Errorf("INBOX holds %d messages, want 3", len(inbox))
	}

	sent, err := s.ListMessages(ctx, folderByPath(t, s, acct.ID, "Sent").ID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(Sent) error: %v", err)
	}
	if len(sent) != 2 {
		t.Errorf("Sent holds %d messages, want 2 — special folders sync eagerly", len(sent))
	}

	// An ordinary folder is listed but its headers wait for the user to open
	// it. Fetching all of them up front is what makes a corporate account with
	// eighty folders take minutes to show anything.
	projects, err := s.ListMessages(ctx, folderByPath(t, s, acct.ID, "Projects").ID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(Projects) error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("ordinary folder holds %d messages after initial sync, want 0 (lazy)", len(projects))
	}
}

func TestInitialSyncRecordsSyncState(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 1, 2, 3)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	inbox := folderByPath(t, s, acct.ID, "INBOX")
	if inbox.UIDNext != 4 {
		t.Errorf("UIDNext = %d, want 4", inbox.UIDNext)
	}
	if inbox.HighestModSeq == 0 {
		t.Error("HighestModSeq = 0; M2's delta sync needs it recorded")
	}
	if inbox.LastSyncedAt.IsZero() {
		t.Error("LastSyncedAt was not recorded")
	}
}

func TestInitialSyncIsIdempotent(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 1, 2, 3)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	// This runs on every startup, so repeating it must not multiply anything.
	for i := range 3 {
		if err := eng.InitialSync(ctx, acct); err != nil {
			t.Fatalf("InitialSync() run %d error: %v", i+1, err)
		}
	}

	msgs, err := s.ListMessages(ctx, folderByPath(t, s, acct.ID, "INBOX").ID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("after three syncs INBOX holds %d messages, want 3", len(msgs))
	}

	folders, err := s.ListFolders(ctx, acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(folders) != 1 {
		t.Errorf("after three syncs the account has %d folders, want 1", len(folders))
	}
}

// The acceptance test from the verification doc. Without this the app would
// show — and act on — messages whose UIDs no longer mean anything.
func TestUIDValidityChangeResetsOnlyThatFolder(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addFolder("Sent", []string{"\\Sent"}, 200)
	be.addMessages("INBOX", 1, 2, 3)
	be.addMessages("Sent", 10, 11)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("first InitialSync() error: %v", err)
	}
	inboxID := folderByPath(t, s, acct.ID, "INBOX").ID
	sentID := folderByPath(t, s, acct.ID, "Sent").ID

	before, err := s.ListMessages(ctx, inboxID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(before) != 3 {
		t.Fatalf("precondition failed: INBOX holds %d messages, want 3", len(before))
	}

	// The server recreates INBOX: same name, brand new UID space, different
	// messages behind the same UIDs.
	be.setUIDValidity("INBOX", 999)
	be.clearMessages("INBOX")
	be.addMessages("INBOX", 1, 2)

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("second InitialSync() error: %v", err)
	}

	inbox, err := s.ListMessages(ctx, inboxID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(INBOX) error: %v", err)
	}
	if len(inbox) != 2 {
		t.Errorf("INBOX holds %d messages after the UIDVALIDITY change, want exactly the 2 refetched ones", len(inbox))
	}

	// The sibling's cache is still valid and must survive untouched.
	sent, err := s.ListMessages(ctx, sentID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(Sent) error: %v", err)
	}
	if len(sent) != 2 {
		t.Errorf("sibling folder holds %d messages, want 2 — the reset leaked beyond INBOX", len(sent))
	}

	if got := folderByPath(t, s, acct.ID, "INBOX").UIDValidity; got != 999 {
		t.Errorf("stored INBOX UIDValidity = %d, want 999", got)
	}
	if got := folderByPath(t, s, acct.ID, "Sent").UIDValidity; got != 200 {
		t.Errorf("stored Sent UIDValidity = %d, want it unchanged at 200", got)
	}
}

// A first sync has no stored UIDVALIDITY to compare against. Treating zero as
// "changed" would reset a folder we just created, which is harmless but would
// hide a real bug behind an extra round trip.
func TestFirstSyncDoesNotResetOnAbsentUIDValidity(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 1, 2)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}
	msgs, err := s.ListMessages(ctx, folderByPath(t, s, acct.ID, "INBOX").ID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("INBOX holds %d messages after a first sync, want 2", len(msgs))
	}
}

func TestInitialSyncSurvivesMidSyncConnectionDrop(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addFolder("Sent", []string{"\\Sent"}, 200)
	be.addMessages("INBOX", 1, 2, 3)
	be.addMessages("Sent", 10, 11)
	// Fail once the first folder's messages have been served.
	be.failFetchAfter = 3

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	err := eng.InitialSync(ctx, acct)
	if err == nil {
		t.Fatal("InitialSync() reported success despite a connection drop")
	}
	if got := Classify(err); got != ClassTransient {
		t.Errorf("Classify(err) = %v, want transient so the engine retries", got)
	}

	// Whatever landed before the failure must still be readable. A partial
	// sync should leave usable data rather than rolling the mailbox back to
	// empty — the user gets some mail now and the rest on retry.
	inbox, err := s.ListMessages(ctx, folderByPath(t, s, acct.ID, "INBOX").ID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(inbox) != 3 {
		t.Errorf("INBOX holds %d messages after a partial sync, want the 3 that arrived before the drop", len(inbox))
	}
}

func TestInitialSyncReportsAuthFailureDistinctly(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 1)
	be.fetchErr = errors.New("authentication failed: invalid_grant")

	eng, _, acct := newTestEngine(t, be)

	err := eng.InitialSync(context.Background(), acct)
	if err == nil {
		t.Fatal("InitialSync() ignored an auth failure")
	}
	// An auth failure needs a sign-in prompt, not a silent retry loop against
	// a credential that will never work.
	if got := Classify(err); got != ClassAuth {
		t.Errorf("Classify(err) = %v, want auth", got)
	}
}

func TestInitialSyncSurfacesDialFailure(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	eng := New(s, func(context.Context, int64) (imapx.MailBackend, error) {
		return nil, errors.New("dial tcp: connection refused")
	})

	err = eng.InitialSync(context.Background(), model.Account{ID: 1, Email: "u@example.com"})
	if err == nil {
		t.Fatal("InitialSync() succeeded despite a failed dial")
	}
	if got := Classify(err); got != ClassTransient {
		t.Errorf("Classify(err) = %v, want transient", got)
	}
}

func TestSyncFolderFillsALazyFolderOnDemand(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addFolder("Projects", nil, 300)
	be.addMessages("INBOX", 1)
	be.addMessages("Projects", 50, 51)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()

	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	projects := folderByPath(t, s, acct.ID, "Projects")
	if err := eng.SyncFolder(ctx, acct, projects); err != nil {
		t.Fatalf("SyncFolder() error: %v", err)
	}

	msgs, err := s.ListMessages(ctx, projects.ID, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("Projects holds %d messages after being opened, want 2", len(msgs))
	}
}

func TestClassifyCoversTheFourCases(t *testing.T) {
	cases := []struct {
		err  error
		want ErrorClass
	}{
		{context.Canceled, ClassTransient},
		{context.DeadlineExceeded, ClassTransient},
		{errors.New("dial tcp 10.0.0.1:993: connection refused"), ClassTransient},
		{errors.New("read tcp: connection reset by peer"), ClassTransient},
		{errors.New("imapx: authentication failed for u@example.com"), ClassAuth},
		{errors.New("auth: refreshing access token failed: invalid_grant"), ClassAuth},
		{errors.New("imapx: FETCH headers failed: bad command"), ClassProtocol},
		{errors.New("store: disk quota exceeded"), ClassPermanent},
	}
	for _, tc := range cases {
		if got := Classify(tc.err); got != tc.want {
			t.Errorf("Classify(%q) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// An authentication failure often arrives wrapped in a message that also names
// the command that triggered it. Reading that as a protocol error would retry
// forever against a credential that will never work.
func TestClassifyPrefersAuthOverProtocolWhenBothAppear(t *testing.T) {
	err := errors.New("imapx: FETCH failed: authentication failed")
	if got := Classify(err); got != ClassAuth {
		t.Errorf("Classify() = %v, want auth", got)
	}
}
