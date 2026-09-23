//go:build linux

package paths

import (
	"os"
	"path/filepath"
)

// platformBase follows the XDG Base Directory spec. os.UserConfigDir would
// give $XDG_CONFIG_HOME, but a mail database is data rather than
// configuration, so $XDG_DATA_HOME is the right base.
func platformBase() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}
