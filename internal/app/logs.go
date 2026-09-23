package app

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LogBundle describes an exported log archive, so the UI can show the user
// what they are about to share before they share it.
type LogBundle struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
	// Preview holds the tail of the current log. The user sees what is in the
	// archive before attaching it to anything — a privacy-focused client that
	// asks people to send their logs owes them that much.
	Preview string `json:"preview"`
}

const previewBytes = 16 << 10

// ExportLogs collects the log files into a zip beside them and returns its
// path with a preview of the contents.
//
// The redacting handler already keeps mail content and credentials out of the
// files. This is the second half of the same promise: nothing leaves the
// machine without the user having had the chance to read it.
func (s *MailService) ExportLogs() (LogBundle, error) {
	logDir, err := s.cfg.LogDir()
	if err != nil {
		return LogBundle{}, err
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return LogBundle{}, fmt.Errorf("app: reading the log directory: %w", err)
	}

	archivePath := filepath.Join(logDir,
		fmt.Sprintf("nexus-logs-%s.zip", time.Now().Format("20060102-150405")))

	archive, err := os.Create(archivePath)
	if err != nil {
		return LogBundle{}, fmt.Errorf("app: creating the archive: %w", err)
	}
	defer func() { _ = archive.Close() }()

	zw := zip.NewWriter(archive)
	added := 0

	for _, entry := range entries {
		// Never fold a previous export into the next one.
		if entry.IsDir() || filepath.Ext(entry.Name()) == ".zip" {
			continue
		}
		if err := addToArchive(zw, logDir, entry.Name()); err != nil {
			_ = zw.Close()
			return LogBundle{}, err
		}
		added++
	}

	if err := zw.Close(); err != nil {
		return LogBundle{}, fmt.Errorf("app: finishing the archive: %w", err)
	}

	info, err := archive.Stat()
	if err != nil {
		return LogBundle{}, err
	}

	return LogBundle{
		Path:    archivePath,
		Bytes:   info.Size(),
		Files:   added,
		Preview: tailOfCurrentLog(logDir),
	}, nil
}

func addToArchive(zw *zip.Writer, dir, name string) error {
	src, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("app: opening %s: %w", name, err)
	}
	defer func() { _ = src.Close() }()

	dst, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("app: adding %s to the archive: %w", name, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("app: copying %s: %w", name, err)
	}
	return nil
}

// tailOfCurrentLog returns the last previewBytes of the live log file. A
// failure here is not worth failing the export over: the archive is written
// either way, and the preview is a convenience.
func tailOfCurrentLog(logDir string) string {
	path := filepath.Join(logDir, "nexus.log")

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return ""
	}
	offset := int64(0)
	if info.Size() > previewBytes {
		offset = info.Size() - previewBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(data)
}
