//go:build windows

package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// revealInFileManager opens Explorer with the file selected.
//
// /select, shows the file without running it. The path is passed as a separate
// argument rather than built into a command line, so a filename from a message
// cannot become part of the command.
func revealInFileManager(path string) error {
	if path == "" {
		return fmt.Errorf("app: nothing to show")
	}
	if err := exec.Command("explorer.exe", "/select,"+filepath.Clean(path)).Start(); err != nil {
		return fmt.Errorf("app: opening the file manager: %w", err)
	}
	return nil
}
