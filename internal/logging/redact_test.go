package logging

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Canary values. If any of these reaches the log, the export button becomes a
// way to hand a stranger the user's mail.
const (
	canarySubject  = "CANARY-SUBJECT-9f3a"
	canarySender   = "canary-sender@example.invalid"
	canaryPassword = "CANARY-PASSWORD-7b21"
	canaryToken    = "CANARY-TOKEN-4c8d"
)

func newTestLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})
	return slog.New(NewRedactingHandler(inner)), &buf
}

func assertNoCanaries(t *testing.T, out string) {
	t.Helper()
	for _, canary := range []string{canarySubject, canarySender, canaryPassword, canaryToken} {
		if strings.Contains(out, canary) {
			t.Errorf("log output leaked %q:\n%s", canary, out)
		}
	}
}

func TestForbiddenKeysAreRedacted(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)

	log.Info("sync finished",
		"account_id", 7,
		"folder_path", "INBOX/Faturalar",
		"subject", canarySubject,
		"from_addr", canarySender,
		"password", canaryPassword,
		"refresh_token", canaryToken,
	)

	out := buf.String()
	assertNoCanaries(t, out)

	// The useful fields must survive, or the rule has turned the log into
	// something nobody can debug with.
	if !strings.Contains(out, "account_id") || !strings.Contains(out, `"account_id":7`) {
		t.Errorf("account_id was lost:\n%s", out)
	}
	if !strings.Contains(out, "INBOX/Faturalar") {
		t.Errorf("folder_path was lost:\n%s", out)
	}
}

// The commonest accident is not logging a subject on purpose — it is logging a
// wrapped error that happens to quote one.
func TestSensitiveShapesInFreeTextAreScrubbed(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)

	log.Error("sync failed",
		"err", fmt.Errorf("imapx: authentication failed for %s: %w",
			canarySender, errors.New("auth=Bearer "+canaryToken)),
	)
	log.Info("connected as " + canarySender)

	out := buf.String()
	if strings.Contains(out, canarySender) {
		t.Errorf("an address inside an error message reached the log:\n%s", out)
	}
	if strings.Contains(out, canaryToken) {
		t.Errorf("a bearer token inside an error message reached the log:\n%s", out)
	}
	if !strings.Contains(out, "authentication failed") {
		t.Errorf("scrubbing removed the diagnosis along with the address:\n%s", out)
	}
}

// Debug mode is where a developer reaches for more detail, which is exactly
// when the rule is most likely to be relaxed by accident.
func TestRedactionAppliesAtDebugLevel(t *testing.T) {
	log, buf := newTestLogger(slog.LevelDebug)

	log.Debug("fetching",
		"subject", canarySubject,
		"to", canarySender,
		"access_token", canaryToken,
	)

	assertNoCanaries(t, buf.String())
}

func TestRedactionSurvivesWithAttrsAndGroups(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)

	// Attributes attached once and reused are the easiest place for a secret
	// to hide, because the call site that logs looks innocent.
	scoped := log.With("account_email", canarySender, "account_id", 3)
	scoped.Info("starting")

	log.Info("nested", slog.Group("account",
		"id", 4,
		"subject", canarySubject,
		"token", canaryToken,
	))

	out := buf.String()
	assertNoCanaries(t, out)
	if !strings.Contains(out, `"account_id":3`) {
		t.Errorf("a non-sensitive attribute was lost:\n%s", out)
	}
}

func TestNonSensitiveValuesPassThroughUnchanged(t *testing.T) {
	log, buf := newTestLogger(slog.LevelInfo)

	log.Info("fetched headers",
		"account_id", 12,
		"folder_path", "INBOX",
		"uid_start", 4100,
		"uid_end", 5100,
		"count", 1000,
		"duration_ms", 842,
		"error_class", "transient",
	)

	out := buf.String()
	for _, want := range []string{
		`"account_id":12`, `"folder_path":"INBOX"`, `"count":1000`,
		`"error_class":"transient"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %s in:\n%s", want, out)
		}
	}
}

func TestSetupWritesToARotatingFile(t *testing.T) {
	dir := t.TempDir()

	log, closeLog, err := Setup(dir, false)
	if err != nil {
		t.Fatalf("Setup() error: %v", err)
	}
	log.Info("hello", "account_id", 1, "subject", canarySubject)
	if err := closeLog(); err != nil {
		t.Fatalf("close: %v", err)
	}

	raw, err := os.ReadFile(LogPath(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(raw)

	if !strings.Contains(out, `"account_id":1`) {
		t.Errorf("the log file is missing the record:\n%s", out)
	}
	// The file is the thing users attach to bug reports, so the guarantee has
	// to hold on disk and not only in the handler's unit tests.
	assertNoCanaries(t, out)
}

func TestSetupCreatesTheLogDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")

	_, closeLog, err := Setup(dir, false)
	if err != nil {
		t.Fatalf("Setup() error: %v", err)
	}
	t.Cleanup(func() { _ = closeLog() })

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Setup() did not create %s: %v", dir, err)
	}
}
