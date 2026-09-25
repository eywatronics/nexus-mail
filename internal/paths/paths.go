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

// AttachmentDir returns where downloaded attachments are kept, creating it if
// necessary.
//
// Inside the data directory rather than the user's Downloads folder: these are
// a cache of what the server already holds, and the retention purge is free to
// remove them. A file the user deliberately saved is a different act, and goes
// wherever they choose.
func AttachmentDir() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(data, "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// OutboxDir returns where messages waiting to be sent are kept, creating it if
// necessary.
//
// On disk rather than in the database, and the queue row holds only a
// reference. A message with a twenty-megabyte attachment is not a thing to put
// in a text column: every read of the operations table would carry it, and
// SQLite would rewrite the whole row on each retry.
//
// Inside the data directory, and not cleaned by the retention purge. Unlike
// attachments these are not a cache of what a server already holds — until the
// message is sent, this file is the only copy in existence.
func OutboxDir() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(data, "outbox")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
