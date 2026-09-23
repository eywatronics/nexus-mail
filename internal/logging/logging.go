// Package logging sets up structured, rotating logs that never carry mail
// content or credentials.
//
// The privacy rule is not decoration. A client whose selling point is that it
// does not leak has no business writing subjects and addresses into a file the
// user is then encouraged to attach to a bug report.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

const logFileName = "nexus.log"

// Rotation limits. Three backups of ten megabytes is enough to cover a sync
// problem that started yesterday, and little enough that nobody notices the
// disk usage.
const (
	maxSizeMB  = 10
	maxBackups = 3
	maxAgeDays = 28
)

// Setup builds the application logger and returns it with a close function.
//
// Output goes to both the log file and stderr: the file is what a user
// attaches to an issue, stderr is what a developer watches while working.
func Setup(logDir string, debug bool) (*slog.Logger, func() error, error) {
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("logging: creating %s: %w", logDir, err)
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, logFileName),
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   true,
	}

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(
		io.MultiWriter(rotator, os.Stderr),
		&slog.HandlerOptions{Level: level},
	)

	return slog.New(NewRedactingHandler(handler)), rotator.Close, nil
}

// LogPath returns where the current log file lives, for the export button.
func LogPath(logDir string) string {
	return filepath.Join(logDir, logFileName)
}
