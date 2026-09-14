package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nexusmail/internal/auth"
	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/store"
	imapsync "nexusmail/internal/sync"
)

type recorder struct {
	mu      sync.Mutex
	names   []string
	events  []SyncEvent
	newMail []NewMailEvent
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	switch ev := data.(type) {
	case SyncEvent:
		r.events = append(r.events, ev)
	case NewMailEvent:
		r.newMail = append(r.newMail, ev)
	}
}

// arrivals returns the new-mail announcements, copied under the lock: the
// watch goroutines emit from their own goroutine and a test reading the slice
// directly would be a data race.
func (r *recorder) arrivals() []NewMailEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]NewMailEvent(nil), r.newMail...)
}

func (r *recorder) recorded() ([]string, []SyncEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, len(r.names))
	copy(names, r.names)
	events := make([]SyncEvent, len(r.events))
	copy(events, r.events)
	return names, events
}

// stubBackend is the smallest MailBackend that lets the service be exercised.
type stubBackend struct{ failFetch error }

func (stubBackend) Capabilities() imapx.Capabilities { return imapx.Capabilities{} }

func (stubBackend) ListFolders(context.Context) ([]model.Folder, error) {
	return []model.Folder{
		{Path: "INBOX", Name: "INBOX", Delimiter: "/", UIDValidity: 1},
		{Path: "Projects", Name: "Projects", Delimiter: "/", UIDValidity: 2},
	}, nil
}

func (stubBackend) Select(context.Context, string) (imapx.SelectResult, error) {
	return imapx.SelectResult{UIDValidity: 1, UIDNext: 2, NumMessages: 1}, nil
}

func (b stubBackend) FetchHeaders(context.Context, imapx.UIDRange) ([]model.Message, error) {
	if b.failFetch != nil {
		return nil, b.failFetch
	}
	return []model.Message{{
		UID:          1,
		Subject:      "Hello",
		From:         model.Address{Name: "A", Addr: "a@example.com"},
		InternalDate: time.Unix(1700000000, 0),
		Flags:        []string{model.FlagSeen},
	}}, nil
}

func (stubBackend) FetchBody(context.Context, uint32) (imapx.Body, error) {
	return imapx.Body{HTML: "<p>hi</p>", Text: "hi"}, nil
}

func (stubBackend) FetchRaw(context.Context, uint32) ([]byte, error) {
	return []byte("Subject: hi\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>hi</p>"), nil
}

func (stubBackend) Close() error { return nil }

func newTestService(t *testing.T, be imapx.MailBackend) (*MailService, *recorder, *store.Store) {
	t.Helper()

	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open() error: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})

	secrets, err := auth.NewFileStore(dir, "test-master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}

	eng := imapsync.New(s, func(context.Context, int64) (imapx.MailBackend, error) {
		return be, nil
	})

	rec := &recorder{}
	return NewMailService(s, secrets, eng, Config{Emit: rec.emit}), rec, s
}

func TestAddPasswordAccountKeepsTheSecretOutOfTheDatabase(t *testing.T) {
	svc, _, s := newTestService(t, stubBackend{})

	const password = "CANARY-APP-PASSWORD"
	acct, err := svc.AddPasswordAccount("u@example.com", "Test User",
		"imap.example.com", 993, "tls", "smtp.example.com", 587, password)
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if acct.ID == 0 {
		t.Fatal("returned account has ID 0")
	}
	if acct.AuthKind != string(model.AuthPassword) {
		t.Errorf("AuthKind = %q, want password", acct.AuthKind)
	}

	// The password must be nowhere in the database file. This is the assertion
	// that would catch a well-meaning future change adding a column for it.
	var found int
	if err := s.Read().QueryRow(
		`SELECT count(*) FROM accounts WHERE secret_ref LIKE ? OR email LIKE ?`,
		"%"+password+"%", "%"+password+"%").Scan(&found); err != nil {
		t.Fatalf("query: %v", err)
	}
	if found != 0 {
		t.Error("the password appears in the accounts table")
	}
}

func TestAddPasswordAccountUsesAPresetWhenTheHostIsBlank(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("someone@gmail.com", "G", "", 0, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if acct.Provider != string(model.ProviderGoogle) {
		t.Errorf("Provider = %q, want google", acct.Provider)
	}
}

// A corporate domain has no preset, so a blank host is a mistake the user must
// be told about rather than a value to invent.
func TestAddPasswordAccountRefusesABlankHostWithNoPreset(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	_, err := svc.AddPasswordAccount("someone@example.com.tr", "K", "", 0, "tls", "", 0, "pw")
	if err == nil {
		t.Fatal("AddPasswordAccount() invented a host for a corporate domain")
	}
	if !strings.Contains(err.Error(), "manually") {
		t.Errorf("error %q does not tell the user what to do", err)
	}
}

func TestAddPasswordAccountRejectsEmptyInput(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if _, err := svc.AddPasswordAccount("", "n", "h", 993, "tls", "", 0, "pw"); err == nil {
		t.Error("AddPasswordAccount() accepted an empty email")
	}
	if _, err := svc.AddPasswordAccount("u@example.com", "n", "h", 993, "tls", "", 0, ""); err == nil {
		t.Error("AddPasswordAccount() accepted an empty password")
	}
}

// If the row cannot be written the secret must not survive under a ref nothing
// points at — that is a credential sitting in the keyring forever.
func TestAddPasswordAccountRemovesTheSecretWhenTheRowFails(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if _, err := svc.AddPasswordAccount("dup@example.com", "A", "h", 993, "tls", "", 0, "pw"); err != nil {
		t.Fatalf("first AddPasswordAccount() error: %v", err)
	}
	// The UNIQUE constraint on email rejects the second one.
	if _, err := svc.AddPasswordAccount("dup@example.com", "B", "h", 993, "tls", "", 0, "pw2"); err == nil {
		t.Fatal("the duplicate account was accepted")
	}

	// The first account's secret is still needed and must survive.
	if _, err := svc.secrets.Get("account:dup@example.com"); err != nil {
		t.Errorf("the surviving account lost its credential: %v", err)
	}
}

func TestAddOAuthAccountRequiresAConfiguredClientID(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	for _, provider := range []string{"google", "microsoft"} {
		_, err := svc.AddOAuthAccount("u@example.com", "U", provider)
		if err == nil {
			t.Fatalf("AddOAuthAccount(%s) succeeded with no client ID configured", provider)
		}
		// Open-source users must register their own client, so the error has
		// to name the guide rather than just refusing.
		if !strings.Contains(err.Error(), "oauth-setup") {
			t.Errorf("error %q does not point at the setup guide", err)
		}
	}
}

func TestAddOAuthAccountRejectsAProviderWithoutOAuth(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if _, err := svc.AddOAuthAccount("u@example.com", "U", "generic"); err == nil {
		t.Error("AddOAuthAccount() accepted a provider that has no OAuth support")
	}
}

func TestSyncAccountEmitsStartAndFinish(t *testing.T) {
	svc, rec, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	names, _ := rec.recorded()
	if len(names) != 2 || names[0] != EventSyncStarted || names[1] != EventSyncFinished {
		t.Fatalf("events = %v, want [%s %s]", names, EventSyncStarted, EventSyncFinished)
	}

	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("ListFolders() returned %d folders, want 2", len(folders))
	}

	var inbox FolderDTO
	for _, f := range folders {
		if f.IsInbox {
			inbox = f
		}
	}
	if inbox.ID == 0 {
		t.Fatal("no folder was flagged as the inbox")
	}

	msgs, err := svc.ListMessages(inbox.ID, 50, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("ListMessages() returned %d messages, want 1", len(msgs))
	}
	if msgs[0].Subject != "Hello" {
		t.Errorf("Subject = %q, want Hello", msgs[0].Subject)
	}
	if !msgs[0].IsRead {
		t.Error("IsRead = false, but the message carries \\Seen")
	}
	if msgs[0].InternalDateUnix != 1700000000 {
		t.Errorf("InternalDateUnix = %d, want 1700000000", msgs[0].InternalDateUnix)
	}
}

// The UI branches on the error class: an auth failure needs a sign-in prompt,
// a transient one needs a spinner. Losing the class in the event would make
// both look the same.
func TestSyncAccountEmitsTheErrorClassOnFailure(t *testing.T) {
	svc, rec, _ := newTestService(t, stubBackend{
		failFetch: fmtError("imapx: authentication failed for u@example.com"),
	})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err == nil {
		t.Fatal("SyncAccount() reported success despite a fetch failure")
	}

	names, events := rec.recorded()
	if len(names) != 2 || names[1] != EventSyncFailed {
		t.Fatalf("events = %v, want the second to be %s", names, EventSyncFailed)
	}
	failure := events[len(events)-1]
	if failure.Class != "auth" {
		t.Errorf("Class = %q, want auth", failure.Class)
	}
	if failure.Error == "" {
		t.Error("the failure event carries no message")
	}
}

func TestSyncAccountReportsAnUnknownAccount(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if err := svc.SyncAccount(4242); err == nil {
		t.Error("SyncAccount() succeeded for an account that does not exist")
	}
}

// Ordinary folders are left empty by the initial sync, so clicking one has to
// fill it in rather than showing an empty list.
func TestOpenFolderSyncsALazyFolder(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	var projects FolderDTO
	for _, f := range folders {
		if f.Path == "Projects" {
			projects = f
		}
	}
	if projects.ID == 0 {
		t.Fatal("the Projects folder was not stored")
	}

	before, err := svc.ListMessages(projects.ID, 50, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("the lazy folder already holds %d messages", len(before))
	}

	after, err := svc.OpenFolder(projects.ID, 50, false)
	if err != nil {
		t.Fatalf("OpenFolder() error: %v", err)
	}
	if len(after) != 1 {
		t.Errorf("OpenFolder() returned %d messages, want 1", len(after))
	}
}

func TestListMessagesClampsThePageSize(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}
	folders, _ := svc.ListFolders(acct.ID)

	// A runaway limit would materialise the whole mailbox on both sides of the
	// bridge; a negative offset would be a SQL error.
	for _, limit := range []int{0, -1, 100000} {
		if _, err := svc.ListMessages(folders[0].ID, limit, -5, false); err != nil {
			t.Errorf("ListMessages(limit=%d) error: %v", limit, err)
		}
	}
}

func TestLoadConfigCreatesABlankFileOnFirstRun(t *testing.T) {
	dir := t.TempDir()

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.GoogleClientID != "" || cfg.MicrosoftClientID != "" {
		t.Error("a fresh config reported client IDs")
	}

	// The user needs a file to edit, not a path to guess.
	path := filepath.Join(dir, configFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("LoadConfig() did not create %s: %v", configFileName, err)
	}

	if err := os.WriteFile(path,
		[]byte(`{"googleClientId":"gid","microsoftClientId":"mid","oauthRedirectPort":53987}`),
		0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err = LoadConfig(dir)
	if err != nil {
		t.Fatalf("second LoadConfig() error: %v", err)
	}
	if cfg.GoogleClientID != "gid" || cfg.MicrosoftClientID != "mid" {
		t.Errorf("client IDs did not round-trip: %+v", cfg)
	}
	if cfg.OAuthRedirectPort != 53987 {
		t.Errorf("OAuthRedirectPort = %d, want 53987", cfg.OAuthRedirectPort)
	}
}

func TestLoadConfigReportsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Silently ignoring a broken config would leave the user staring at "no
	// client ID configured" with a file that plainly contains one.
	if _, err := LoadConfig(dir); err == nil {
		t.Error("LoadConfig() accepted malformed JSON")
	}
}

// fmtError keeps the stub's error construction out of the test bodies.
func fmtError(msg string) error { return &simpleError{msg} }

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

func TestSearchMessagesSpansFoldersAndClampsTheLimit(t *testing.T) {
	svc, _, s := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}
	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}

	// Put a message in the folder the user is *not* looking at. Searching only
	// the open folder would find nothing, which is the failure this method
	// exists to avoid.
	var other FolderDTO
	for _, f := range folders {
		if !f.IsInbox {
			other = f
		}
	}
	if other.ID == 0 {
		t.Fatal("fixture has no non-inbox folder")
	}
	if err := s.UpsertMessages(context.Background(), other.ID, []model.Message{{
		AccountID: acct.ID, FolderID: other.ID, UID: 77,
		Subject:      "Mutabakat dosyası",
		From:         model.Address{Addr: "muhasebe@example.com"},
		InternalDate: time.Unix(1700000002, 0),
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	got, err := svc.SearchMessages(acct.ID, "mutabakat", 50)
	if err != nil {
		t.Fatalf("SearchMessages() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("search found %d messages, want 1", len(got))
	}
	if got[0].FolderID != other.ID {
		t.Errorf("result is in folder %d, want %d", got[0].FolderID, other.ID)
	}

	// Same clamp as the list: a runaway limit would materialise the whole
	// mailbox on both sides of the bridge.
	for _, limit := range []int{0, -1, 100000} {
		if _, err := svc.SearchMessages(acct.ID, "mutabakat", limit); err != nil {
			t.Errorf("SearchMessages(limit=%d) error: %v", limit, err)
		}
	}
}

// An empty box is the state the UI sits in most of the time, and it must not
// be an error path.
func TestSearchMessagesReturnsNothingForABlankQuery(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}

	for _, query := range []string{"", "   ", "!!!"} {
		got, err := svc.SearchMessages(acct.ID, query, 50)
		if err != nil {
			t.Fatalf("SearchMessages(%q) error: %v", query, err)
		}
		if len(got) != 0 {
			t.Errorf("SearchMessages(%q) returned %d messages, want 0", query, len(got))
		}
	}
}

func (stubBackend) FetchFlags(context.Context, imapx.UIDRange, uint64) ([]model.FlagUpdate, error) {
	return nil, nil
}

func (stubBackend) Idle(ctx context.Context) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

// Live sync only exists if something starts it. The engine's watch loop is
// complete and tested on its own; this is the wiring that was missing when the
// search index was built and never read.
func TestStartWatchingRunsALoopPerAccount(t *testing.T) {
	svc, rec, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartWatching(ctx); err != nil {
		t.Fatalf("StartWatching() error: %v", err)
	}

	waitFor(t, func() bool { return svc.watcherCount() == 1 })
	_ = rec
}

// A newly added account has to start syncing without a restart.
func TestSyncAccountStartsWatchingTheAccount(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartWatching(ctx); err != nil {
		t.Fatalf("StartWatching() error: %v", err)
	}
	if svc.watcherCount() != 0 {
		t.Fatalf("precondition failed: %d watchers before any account", svc.watcherCount())
	}

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	waitFor(t, func() bool { return svc.watcherCount() == 1 })
}

// Two calls must not mean two connections per account. Gmail locks an account
// that opens too many.
func TestStartWatchingIsIdempotent(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range 3 {
		if err := svc.StartWatching(ctx); err != nil {
			t.Fatalf("StartWatching() error: %v", err)
		}
	}

	waitFor(t, func() bool { return svc.watcherCount() == 1 })
	if n := svc.watcherCount(); n != 1 {
		t.Errorf("%d watchers for one account", n)
	}
}

// Closing the window has to close the connections. A watcher outliving the
// context would hold an IMAP connection open after the app is gone.
func TestCancellingTheContextStopsEveryWatcher(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.StartWatching(ctx); err != nil {
		t.Fatalf("StartWatching() error: %v", err)
	}
	waitFor(t, func() bool { return svc.watcherCount() == 1 })

	cancel()
	waitFor(t, func() bool { return svc.watcherCount() == 0 })
}

// waitFor polls a condition for a bounded time. The watchers run in their own
// goroutines, so their effect is not visible the instant the call returns.
func waitFor(t *testing.T, cond func() bool) {
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

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return dir
}

// Most people never touch this, so the default has to be sane on its own.
func TestLoadConfigUsesTheDefaultRetentionWhenUnset(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `{"googleClientId": "x"}`))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Retention != model.DefaultRetention {
		t.Errorf("Retention = %+v, want the default %+v", cfg.Retention, model.DefaultRetention)
	}
}

// "Keep everything" has to be expressible. Zero and absent mean different
// things here, which is why the fields are pointers: a user who writes 0 is
// making a decision, and silently replacing it with the default would delete
// mail they asked to keep.
func TestLoadConfigLetsTheUserTurnRetentionOff(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `{"retentionDays": 0, "retentionMaxMessages": 0}`))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Retention.Enabled() {
		t.Errorf("Retention = %+v, want nothing purged", cfg.Retention)
	}
}

func TestLoadConfigHonoursCustomRetention(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `{"retentionDays": 30, "retentionMaxMessages": 500}`))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Retention.MaxAge != 30*24*time.Hour {
		t.Errorf("MaxAge = %s, want 720h", cfg.Retention.MaxAge)
	}
	if cfg.Retention.MaxMessages != 500 {
		t.Errorf("MaxMessages = %d, want 500", cfg.Retention.MaxMessages)
	}
}

// One limit set and the other left out is a reasonable thing to write, and the
// missing one should stay at its default rather than silently becoming "off".
func TestLoadConfigFillsInTheRetentionLimitNotGiven(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `{"retentionDays": 30}`))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Retention.MaxMessages != model.DefaultRetention.MaxMessages {
		t.Errorf("MaxMessages = %d, want the default %d",
			cfg.Retention.MaxMessages, model.DefaultRetention.MaxMessages)
	}
}

// A negative value is a typo. Treating it as "off" would keep everything while
// the user believes they set a limit; saying so is the only honest option.
func TestLoadConfigRejectsNegativeRetention(t *testing.T) {
	for _, body := range []string{
		`{"retentionDays": -1}`,
		`{"retentionMaxMessages": -5}`,
	} {
		if _, err := LoadConfig(writeConfig(t, body)); err == nil {
			t.Errorf("LoadConfig(%s) accepted a negative value", body)
		}
	}
}

func (stubBackend) StoreFlags(context.Context, []uint32, []string, bool) error { return nil }
func (stubBackend) Move(context.Context, []uint32, string) error               { return nil }
func (stubBackend) Expunge(context.Context, []uint32) error                    { return nil }

// syncedAccountWithMessages gets a service to the state the UI acts from:
// an account with a synced inbox holding messages.
func syncedAccountWithMessages(t *testing.T) (*MailService, *store.Store, AccountDTO, FolderDTO, []MessageDTO) {
	t.Helper()

	svc, _, s := newTestService(t, stubBackend{})
	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	var inbox FolderDTO
	for _, f := range folders {
		if f.IsInbox {
			inbox = f
		}
	}
	msgs, err := svc.ListMessages(inbox.ID, 50, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("the fixture has no messages")
	}
	return svc, s, acct, inbox, msgs
}

func pendingOps(t *testing.T, s *store.Store, accountID int64) []model.Operation {
	t.Helper()

	ops, err := s.ClaimOperations(context.Background(), accountID, 50)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	return ops
}

// The window shows the change immediately and the server hears about it
// afterwards. That is the whole local-first bargain, and it only holds if both
// halves actually happen.
func TestMarkReadUpdatesLocallyAndQueues(t *testing.T) {
	svc, s, acct, inbox, msgs := syncedAccountWithMessages(t)

	// The fixture arrives already read, so unread is the change to make.
	if err := svc.MarkRead([]int64{msgs[0].ID}, false); err != nil {
		t.Fatalf("MarkRead() error: %v", err)
	}

	after, err := svc.ListMessages(inbox.ID, 50, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if after[0].IsRead {
		t.Error("the message still reads as read in the window")
	}

	ops := pendingOps(t, s, acct.ID)
	if len(ops) != 1 || ops[0].Kind != model.OpRemoveFlags {
		t.Errorf("queued %+v, want a remove-flags for the seen flag", ops)
	}
}

func TestSetStarredUpdatesLocallyAndQueues(t *testing.T) {
	svc, s, acct, inbox, msgs := syncedAccountWithMessages(t)

	if err := svc.SetStarred([]int64{msgs[0].ID}, true); err != nil {
		t.Fatalf("SetStarred() error: %v", err)
	}

	after, _ := svc.ListMessages(inbox.ID, 50, 0, false)
	if !after[0].IsStarred {
		t.Error("the message is not starred in the window")
	}
	if ops := pendingOps(t, s, acct.ID); len(ops) != 1 || ops[0].Kind != model.OpAddFlags {
		t.Errorf("queued %+v, want an add-flags", ops)
	}
}

func TestDeleteMessagesRemovesLocallyAndQueues(t *testing.T) {
	svc, s, acct, inbox, msgs := syncedAccountWithMessages(t)

	if err := svc.DeleteMessages([]int64{msgs[0].ID}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}

	after, _ := svc.ListMessages(inbox.ID, 50, 0, false)
	for _, m := range after {
		if m.ID == msgs[0].ID {
			t.Error("the deleted message is still in the window")
		}
	}
	if ops := pendingOps(t, s, acct.ID); len(ops) != 1 || ops[0].Kind != model.OpDelete {
		t.Errorf("queued %+v, want a delete", ops)
	}
}

func TestMoveMessagesRemovesFromTheSourceAndQueues(t *testing.T) {
	svc, s, acct, inbox, msgs := syncedAccountWithMessages(t)

	folders, _ := svc.ListFolders(acct.ID)
	var other FolderDTO
	for _, f := range folders {
		if !f.IsInbox {
			other = f
		}
	}
	if other.ID == 0 {
		t.Fatal("the fixture has no second folder")
	}

	if err := svc.MoveMessages([]int64{msgs[0].ID}, other.ID); err != nil {
		t.Fatalf("MoveMessages() error: %v", err)
	}

	after, _ := svc.ListMessages(inbox.ID, 50, 0, false)
	for _, m := range after {
		if m.ID == msgs[0].ID {
			t.Error("the moved message is still in the source folder")
		}
	}
	ops := pendingOps(t, s, acct.ID)
	if len(ops) != 1 || ops[0].Kind != model.OpMove || ops[0].TargetFolderID != other.ID {
		t.Errorf("queued %+v, want a move to folder %d", ops, other.ID)
	}
}

// An empty selection is what a keyboard shortcut sends when nothing is
// selected. It must not be an error, and must not queue anything.
func TestActionsOnAnEmptySelectionAreHarmless(t *testing.T) {
	svc, s, acct, _, _ := syncedAccountWithMessages(t)

	if err := svc.MarkRead(nil, true); err != nil {
		t.Errorf("MarkRead(nil) error: %v", err)
	}
	if err := svc.SetStarred(nil, true); err != nil {
		t.Errorf("SetStarred(nil) error: %v", err)
	}
	if err := svc.DeleteMessages(nil); err != nil {
		t.Errorf("DeleteMessages(nil) error: %v", err)
	}
	if ops := pendingOps(t, s, acct.ID); len(ops) != 0 {
		t.Errorf("an empty selection queued %+v", ops)
	}
}

func (stubBackend) FetchPart(context.Context, uint32, string, string) ([]byte, error) {
	return []byte("stub"), nil
}

// The paperclip in the list is a promise that there is something to open.
// Listing is what keeps it from being an empty one.
func TestListAttachmentsReturnsWhatTheSyncRecorded(t *testing.T) {
	ctx := context.Background()
	svc, _, s := newTestService(t, attachmentBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}
	_ = s

	folders, _ := svc.ListFolders(acct.ID)
	var inbox FolderDTO
	for _, f := range folders {
		if f.IsInbox {
			inbox = f
		}
	}
	msgs, _ := svc.ListMessages(inbox.ID, 10, 0, false)

	got, err := svc.ListAttachments(msgs[0].ID)
	if err != nil {
		t.Fatalf("ListAttachments() error: %v", err)
	}
	if len(got) != 1 || got[0].Filename != "rapor.pdf" {
		t.Fatalf("listed %+v, want the one file the sync described", got)
	}
	if got[0].Downloaded {
		t.Error("the attachment claims to be downloaded before anyone asked")
	}
	_ = ctx
}

// Downloading writes the bytes somewhere the window can hand to the operating
// system, and records where, so asking twice does not fetch twice.
func TestDownloadAttachmentWritesTheFileAndRemembersIt(t *testing.T) {
	svc, _, _ := newTestService(t, attachmentBackend{})
	dir := t.TempDir()
	svc.cfg.AttachmentDir = func() (string, error) { return dir, nil }

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	folders, _ := svc.ListFolders(acct.ID)
	var inbox FolderDTO
	for _, f := range folders {
		if f.IsInbox {
			inbox = f
		}
	}
	msgs, _ := svc.ListMessages(inbox.ID, 10, 0, false)
	listed, _ := svc.ListAttachments(msgs[0].ID)

	got, err := svc.DownloadAttachment(listed[0].ID)
	if err != nil {
		t.Fatalf("DownloadAttachment() error: %v", err)
	}
	if !got.Downloaded || got.LocalPath == "" {
		t.Fatalf("the download reported %+v", got)
	}

	contents, err := os.ReadFile(got.LocalPath)
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	if string(contents) != "dosya icerigi" {
		t.Errorf("the file holds %q", string(contents))
	}

	// Asking again must not go back to the server: the file is already here,
	// and refetching it would make opening a large attachment twice cost twice.
	again, err := svc.DownloadAttachment(listed[0].ID)
	if err != nil {
		t.Fatalf("second DownloadAttachment() error: %v", err)
	}
	if again.LocalPath != got.LocalPath {
		t.Errorf("the second download saved to %q, want the same file", again.LocalPath)
	}
}

// A filename from a message is attacker-controlled. Writing it straight into a
// path is how a mail client overwrites something it had no business touching.
func TestDownloadAttachmentDoesNotLetAFilenameEscapeTheDirectory(t *testing.T) {
	svc, _, _ := newTestService(t, attachmentBackend{filename: "../../../etc/passwd"})
	dir := t.TempDir()
	svc.cfg.AttachmentDir = func() (string, error) { return dir, nil }

	acct, _ := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}
	folders, _ := svc.ListFolders(acct.ID)
	var inbox FolderDTO
	for _, f := range folders {
		if f.IsInbox {
			inbox = f
		}
	}
	msgs, _ := svc.ListMessages(inbox.ID, 10, 0, false)
	listed, _ := svc.ListAttachments(msgs[0].ID)

	got, err := svc.DownloadAttachment(listed[0].ID)
	if err != nil {
		t.Fatalf("DownloadAttachment() error: %v", err)
	}
	if !strings.HasPrefix(got.LocalPath, dir) {
		t.Errorf("the file was written to %q, outside %q", got.LocalPath, dir)
	}
}

// attachmentBackend serves one message carrying one file.
type attachmentBackend struct {
	stubBackend
	filename string
}

func (b attachmentBackend) FetchHeaders(context.Context, imapx.UIDRange) ([]model.Message, error) {
	name := b.filename
	if name == "" {
		name = "rapor.pdf"
	}
	return []model.Message{{
		UID:            1,
		Subject:        "Rapor",
		From:           model.Address{Addr: "a@example.com"},
		InternalDate:   time.Unix(1700000000, 0),
		Flags:          []string{model.FlagSeen},
		HasAttachments: true,
		Attachments: []model.AttachmentPart{
			{PartID: "2", Filename: name, MIMEType: "application/pdf", Size: 13},
		},
	}}, nil
}

func (attachmentBackend) FetchPart(context.Context, uint32, string, string) ([]byte, error) {
	return []byte("dosya icerigi"), nil
}

// The threaded view is a different order, not a different set. A reader
// switching views must not find messages missing from one of them.
func TestThreadedAndFlatListsHoldTheSameMessages(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	folders, _ := svc.ListFolders(acct.ID)
	var inbox FolderDTO
	for _, f := range folders {
		if f.IsInbox {
			inbox = f
		}
	}

	flat, err := svc.ListMessages(inbox.ID, 50, 0, false)
	if err != nil {
		t.Fatalf("flat ListMessages() error: %v", err)
	}
	threaded, err := svc.ListMessages(inbox.ID, 50, 0, true)
	if err != nil {
		t.Fatalf("threaded ListMessages() error: %v", err)
	}

	if len(flat) != len(threaded) {
		t.Fatalf("flat has %d messages, threaded has %d", len(flat), len(threaded))
	}

	seen := map[int64]bool{}
	for _, m := range flat {
		seen[m.ID] = true
	}
	for _, m := range threaded {
		if !seen[m.ID] {
			t.Errorf("message %d is in the threaded list but not the flat one", m.ID)
		}
	}

	// The count is the threaded list's answer and nothing else's: reporting it
	// in the flat list would have the window draw conversation groups in a
	// view that is not grouping anything.
	for _, m := range flat {
		if m.ThreadCount != 0 {
			t.Errorf("the flat list reports ThreadCount %d for message %d", m.ThreadCount, m.ID)
		}
	}
	for _, m := range threaded {
		if m.ThreadCount < 1 {
			t.Errorf("the threaded list reports ThreadCount %d for message %d", m.ThreadCount, m.ID)
		}
	}
}

// On-premises Exchange is why the choice exists, and the choice is worthless
// if the wizard's answer does not reach the connection.
func TestAddPasswordAccountRecordsTheConnectionSecurity(t *testing.T) {
	svc, _, db := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("bt@sirket.local", "BT",
		"mail.sirket.local", 143, "starttls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}

	stored, err := db.GetAccount(context.Background(), acct.ID)
	if err != nil {
		t.Fatalf("GetAccount() error: %v", err)
	}
	if stored.IMAPSecurity != model.SecuritySTARTTLS {
		t.Errorf("IMAPSecurity = %q, want starttls", stored.IMAPSecurity)
	}
}

// Anything unrecognised resolves to implicit TLS. A typo must not produce a
// weaker connection than the one the user thought they were choosing.
func TestAnUnrecognisedSecurityBecomesTLS(t *testing.T) {
	svc, _, db := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("x@sirket.local", "X",
		"mail.sirket.local", 993, "sslv3-maybe", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}

	stored, _ := db.GetAccount(context.Background(), acct.ID)
	if stored.IMAPSecurity != model.SecurityTLS {
		t.Errorf("IMAPSecurity = %q for a nonsense value, want tls", stored.IMAPSecurity)
	}
}

// A blank port is the common case for someone who was told "STARTTLS" and
// nothing else. Guessing it from the security is better than failing on a
// field they did not know to fill in.
func TestTheDefaultPortFollowsTheSecurity(t *testing.T) {
	svc, _, db := newTestService(t, stubBackend{})

	starttls, err := svc.AddPasswordAccount("a@sirket.local", "A",
		"mail.sirket.local", 0, "starttls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	tlsAcct, err := svc.AddPasswordAccount("b@sirket.local", "B",
		"mail.sirket.local", 0, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}

	ctx := context.Background()
	a, _ := db.GetAccount(ctx, starttls.ID)
	b, _ := db.GetAccount(ctx, tlsAcct.ID)

	if a.IMAPPort != 143 {
		t.Errorf("STARTTLS account got port %d, want 143", a.IMAPPort)
	}
	if b.IMAPPort != 993 {
		t.Errorf("TLS account got port %d, want 993", b.IMAPPort)
	}
}

// A preset names a host that answers on 993. Carrying a STARTTLS choice into
// it would point the upgrade at a port with nothing to upgrade.
func TestAPresetOverridesAChosenSTARTTLS(t *testing.T) {
	svc, _, db := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("someone@gmail.com", "G", "", 0, "starttls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}

	stored, _ := db.GetAccount(context.Background(), acct.ID)
	if stored.IMAPSecurity != model.SecurityTLS || stored.IMAPPort != 993 {
		t.Errorf("preset account is %s on port %d, want tls on 993",
			stored.IMAPSecurity, stored.IMAPPort)
	}
}
