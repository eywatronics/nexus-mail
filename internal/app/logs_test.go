package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportLogsBundlesTheFilesAndShowsThemFirst(t *testing.T) {
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "nexus.log"),
		[]byte(`{"level":"INFO","msg":"sync finished","account_id":1}`+"\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "nexus-2026-09-08.log"),
		[]byte(`{"level":"INFO","msg":"older"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write rotated log: %v", err)
	}

	svc, _, _ := newTestService(t, stubBackend{})
	svc.cfg.LogDir = func() (string, error) { return logDir, nil }

	bundle, err := svc.ExportLogs()
	if err != nil {
		t.Fatalf("ExportLogs() error: %v", err)
	}
	if bundle.Files != 2 {
		t.Errorf("Files = %d, want 2", bundle.Files)
	}
	if bundle.Bytes == 0 {
		t.Error("the archive is empty")
	}

	// The user must be able to read what they are about to share.
	if !strings.Contains(bundle.Preview, "sync finished") {
		t.Errorf("Preview = %q, want the tail of the current log", bundle.Preview)
	}

	zr, err := zip.OpenReader(bundle.Path)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = zr.Close() }()

	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["nexus.log"] || !names["nexus-2026-09-08.log"] {
		t.Errorf("archive holds %v, want both log files", names)
	}
}

// Exporting twice must not fold the first archive into the second, which would
// double the size on every attempt.
func TestExportLogsSkipsPreviousArchives(t *testing.T) {
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "nexus.log"), []byte("line\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	svc, _, _ := newTestService(t, stubBackend{})
	svc.cfg.LogDir = func() (string, error) { return logDir, nil }

	if _, err := svc.ExportLogs(); err != nil {
		t.Fatalf("first ExportLogs() error: %v", err)
	}
	second, err := svc.ExportLogs()
	if err != nil {
		t.Fatalf("second ExportLogs() error: %v", err)
	}
	if second.Files != 1 {
		t.Errorf("Files = %d on the second export, want 1", second.Files)
	}
}
