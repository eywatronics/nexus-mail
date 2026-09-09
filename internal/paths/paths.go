// Package paths resolves per-platform locations for user data.
package paths

import (
	"os"
	"path/filepath"
)

const appDirName = "nexus-mail"

// DataDir returns the directory holding the database, attachments, config and
// logs, creating it if necessary.
//
//	Windows  %APPDATA%\nexus-mail\
//	macOS    ~/Library/Application Support/nexus-mail/
//	Linux    $XDG_DATA_HOME/nexus-mail/  (or ~/.local/share/nexus-mail/)
//
// Mode 0700: the database holds mail, and no other user on the machine has any
// business reading it.
func DataDir() (string, error) {
	base, err := platformBase()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// LogDir returns the log directory, creating it if necessary.
func LogDir() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(data, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
