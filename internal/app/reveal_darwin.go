//go:build darwin

package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// revealInFileManager opens Finder with the file selected.
func revealInFileManager(path string) error {
	if path == "" {
		return fmt.Errorf("app: nothing to show")
	}
	if err := exec.Command("open", "-R", filepath.Clean(path)).Start(); err != nil {
		return fmt.Errorf("app: opening the file manager: %w", err)
	}
	return nil
}
