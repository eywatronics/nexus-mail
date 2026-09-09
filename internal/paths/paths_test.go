package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirIsCreatedAndNamed(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	t.Setenv("APPDATA", base)

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}
	if !strings.HasSuffix(filepath.Clean(dir), appDirName) {
		t.Errorf("expected path to end with %q, got %q", appDirName, dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected a directory, got a file")
	}
}

func TestDataDirIsIdempotent(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	t.Setenv("APPDATA", base)

	first, err := DataDir()
	if err != nil {
		t.Fatalf("first DataDir() error: %v", err)
	}
	// A second call on an existing directory must succeed rather than failing
	// on "already exists"; it runs on every startup.
	second, err := DataDir()
	if err != nil {
		t.Fatalf("second DataDir() error: %v", err)
	}
	if first != second {
		t.Errorf("DataDir() returned %q then %q; it must be stable", first, second)
	}
}

func TestLogDirLivesUnderDataDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	t.Setenv("APPDATA", base)

	data, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error: %v", err)
	}
	logs, err := LogDir()
	if err != nil {
		t.Fatalf("LogDir() error: %v", err)
	}
	if filepath.Dir(filepath.Clean(logs)) != filepath.Clean(data) {
		t.Errorf("LogDir() = %q, want it directly under %q", logs, data)
	}
	if _, err := os.Stat(logs); err != nil {
		t.Errorf("expected the log directory to exist: %v", err)
	}
}
