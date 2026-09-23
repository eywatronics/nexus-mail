//go:build !linux

package paths

import "os"

// platformBase uses the OS convention: %APPDATA% on Windows and
// ~/Library/Application Support on macOS.
func platformBase() (string, error) {
	return os.UserConfigDir()
}
