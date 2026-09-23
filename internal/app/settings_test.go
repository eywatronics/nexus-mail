package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// settingsFixture gives a service whose config.json lives in a temporary
// directory, so saving is real rather than mocked.
func settingsFixture(t *testing.T) (*MailService, string) {
	t.Helper()

	dir := t.TempDir()
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	svc, _, _ := newTestService(t, stubBackend{})
	svc.cfg.DataDir = dir
	svc.liveRetention = cfg.Retention
	svc.liveNotificationPreview = cfg.NotificationPreview
	svc.liveUndoWindow = cfg.UndoWindow

	return svc, dir
}

func readConfigFile(t *testing.T, dir string) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}
	return out
}

// The OAuth client id is the first thing a new user has to set, and the answer
// used to be "open a JSON file in a directory the app never names".
func TestSettingsSurviveARoundTripThroughTheFile(t *testing.T) {
	svc, dir := settingsFixture(t)

	next := SettingsDTO{
		GoogleClientID:       "google-client",
		MicrosoftClientID:    "microsoft-client",
		OAuthRedirectPort:    9876,
		RetentionDays:        30,
		RetentionMaxMessages: 500,
		NotificationPreview:  false,
		UndoWindowSeconds:    10,
	}
	if err := svc.UpdateSettings(next); err != nil {
		t.Fatalf("UpdateSettings() error: %v", err)
	}

	// What the window would show.
	if got := svc.Settings(); got != next {
		t.Errorf("Settings() = %+v, want %+v", got, next)
	}

	// And what a restart would read.
	reloaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if reloaded.MicrosoftClientID != "microsoft-client" {
		t.Errorf("the client id did not survive: %q", reloaded.MicrosoftClientID)
	}
	if reloaded.UndoWindow != 10*time.Second {
		t.Errorf("UndoWindow = %s after reload, want 10s", reloaded.UndoWindow)
	}
	if reloaded.NotificationPreview {
		t.Error("the notification preview came back on after a reload")
	}
	if reloaded.Retention.MaxMessages != 500 {
		t.Errorf("MaxMessages = %d after reload", reloaded.Retention.MaxMessages)
	}
}

// A setting the running app ignores until a restart is a setting that looks
// broken. These two have to take effect at once.
func TestChangedSettingsTakeEffectWithoutARestart(t *testing.T) {
	svc, _ := settingsFixture(t)

	if err := svc.UpdateSettings(SettingsDTO{
		UndoWindowSeconds:   0,
		NotificationPreview: false,
		RetentionDays:       1,
	}); err != nil {
		t.Fatalf("UpdateSettings() error: %v", err)
	}

	if svc.undoWindow() != 0 {
		t.Errorf("undoWindow() = %s, want the new value", svc.undoWindow())
	}
	if svc.notificationPreview() {
		t.Error("notificationPreview() still reports the old value")
	}

	if err := svc.UpdateSettings(SettingsDTO{UndoWindowSeconds: 20}); err != nil {
		t.Fatalf("UpdateSettings() error: %v", err)
	}
	if svc.undoWindow() != 20*time.Second {
		t.Errorf("undoWindow() = %s, want 20s", svc.undoWindow())
	}
}

// Clamping a typo to zero would turn "I set a limit" into "keep everything",
// and the user would find out when the disk filled.
func TestNegativeSettingsAreRefusedRatherThanClamped(t *testing.T) {
	svc, dir := settingsFixture(t)
	before := readConfigFile(t, dir)

	cases := []struct {
		name string
		in   SettingsDTO
		want string
	}{
		{"retention days", SettingsDTO{RetentionDays: -1}, "retentionDays"},
		{"retention messages", SettingsDTO{RetentionMaxMessages: -5}, "retentionMaxMessages"},
		{"undo window", SettingsDTO{UndoWindowSeconds: -3}, "undoWindowSeconds"},
		{"redirect port", SettingsDTO{OAuthRedirectPort: 70000}, "oauthRedirectPort"},
	}
	for _, c := range cases {
		err := svc.UpdateSettings(c.in)
		if err == nil {
			t.Errorf("%s: UpdateSettings() accepted %+v", c.name, c.in)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not name the field", c.name, err)
		}
	}

	// And nothing was written on the way to being refused.
	if after := readConfigFile(t, dir); len(after) != len(before) {
		t.Errorf("a refused save changed the file: %v then %v", before, after)
	}
}

// Zero is a real answer for all three, and refusing it would take away the
// only way to say "keep everything" or "send it now".
func TestZeroIsAcceptedWhereItMeansSomething(t *testing.T) {
	svc, _ := settingsFixture(t)

	if err := svc.UpdateSettings(SettingsDTO{
		RetentionDays: 0, RetentionMaxMessages: 0, UndoWindowSeconds: 0, OAuthRedirectPort: 0,
	}); err != nil {
		t.Fatalf("UpdateSettings() refused zeroes: %v", err)
	}
	if got := svc.Settings(); got.RetentionDays != 0 || got.UndoWindowSeconds != 0 {
		t.Errorf("Settings() = %+v, want the zeroes kept", got)
	}
}

// A setting that took effect and then failed to save would come back changed
// on the next start — the one outcome worse than not saving at all.
func TestNothingIsAppliedWhenTheFileCannotBeWritten(t *testing.T) {
	svc, _ := settingsFixture(t)
	svc.cfg.DataDir = filepath.Join(t.TempDir(), "does-not-exist")

	before := svc.Settings()
	if err := svc.UpdateSettings(SettingsDTO{UndoWindowSeconds: 30}); err == nil {
		t.Fatal("UpdateSettings() reported success with nowhere to save")
	}
	if got := svc.Settings(); got != before {
		t.Errorf("Settings() = %+v after a failed save, want %+v", got, before)
	}
}

func TestSavingWithNoDataDirectoryIsRefused(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if err := svc.UpdateSettings(SettingsDTO{}); err == nil {
		t.Error("UpdateSettings() succeeded with no directory to save in")
	}
}

// The file is replaced through a temporary neighbour, so a crash halfway
// through cannot leave a config.json the app will not start with.
func TestSavingLeavesNoTemporaryFileBehind(t *testing.T) {
	svc, dir := settingsFixture(t)

	if err := svc.UpdateSettings(SettingsDTO{UndoWindowSeconds: 7}); err != nil {
		t.Fatalf("UpdateSettings() error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the data directory: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("%s was left behind", e.Name())
		}
	}
}

// Settings that cannot take effect until a restart have to be named, or the
// user is left wondering why nothing changed.
func TestTheRestartListNamesTheOAuthSettings(t *testing.T) {
	named := strings.Join(SettingsNeedRestart(), " ")

	for _, want := range []string{"googleClientId", "microsoftClientId", "oauthRedirectPort"} {
		if !strings.Contains(named, want) {
			t.Errorf("the restart list does not mention %s", want)
		}
	}
	// And must not name the ones that do apply at once, which would send
	// people restarting for nothing.
	for _, unwanted := range []string{"undoWindowSeconds", "notificationPreview"} {
		if strings.Contains(named, unwanted) {
			t.Errorf("the restart list mentions %s, which applies immediately", unwanted)
		}
	}
}
