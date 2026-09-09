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
	mu     sync.Mutex
	names  []string
	events []SyncEvent
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	if ev, ok := data.(SyncEvent); ok {
		r.events = append(r.events, ev)
	}
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
		"imap.example.com", 993, "smtp.example.com", 587, password)
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

	acct, err := svc.AddPasswordAccount("someone@gmail.com", "G", "", 0, "", 0, "pw")
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

	_, err := svc.AddPasswordAccount("someone@ozdilek.com.tr", "K", "", 0, "", 0, "pw")
	if err == nil {
		t.Fatal("AddPasswordAccount() invented a host for a corporate domain")
	}
	if !strings.Contains(err.Error(), "manually") {
		t.Errorf("error %q does not tell the user what to do", err)
	}
}

func TestAddPasswordAccountRejectsEmptyInput(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if _, err := svc.AddPasswordAccount("", "n", "h", 993, "", 0, "pw"); err == nil {
		t.Error("AddPasswordAccount() accepted an empty email")
	}
	if _, err := svc.AddPasswordAccount("u@example.com", "n", "h", 993, "", 0, ""); err == nil {
		t.Error("AddPasswordAccount() accepted an empty password")
	}
}

// If the row cannot be written the secret must not survive under a ref nothing
// points at — that is a credential sitting in the keyring forever.
func TestAddPasswordAccountRemovesTheSecretWhenTheRowFails(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if _, err := svc.AddPasswordAccount("dup@example.com", "A", "h", 993, "", 0, "pw"); err != nil {
		t.Fatalf("first AddPasswordAccount() error: %v", err)
	}
	// The UNIQUE constraint on email rejects the second one.
	if _, err := svc.AddPasswordAccount("dup@example.com", "B", "h", 993, "", 0, "pw2"); err == nil {
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

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "", 0, "pw")
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

	msgs, err := svc.ListMessages(inbox.ID, 50, 0)
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

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "", 0, "pw")
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

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "", 0, "pw")
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

	before, err := svc.ListMessages(projects.ID, 50, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("the lazy folder already holds %d messages", len(before))
	}

	after, err := svc.OpenFolder(projects.ID, 50)
	if err != nil {
		t.Fatalf("OpenFolder() error: %v", err)
	}
	if len(after) != 1 {
		t.Errorf("OpenFolder() returned %d messages, want 1", len(after))
	}
}

func TestListMessagesClampsThePageSize(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "", 0, "pw")
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
		if _, err := svc.ListMessages(folders[0].ID, limit, -5); err != nil {
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
