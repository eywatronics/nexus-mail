//go:build !windows && !darwin

package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// revealInFileManager opens the directory holding the file.
//
// The directory rather than the file: xdg-open on a file runs it with whatever
// handler is registered, and an attachment is a stranger's file. There is no
// portable "select this file" on Linux, so the folder is the honest answer.
func revealInFileManager(path string) error {
	if path == "" {
		return fmt.Errorf("app: nothing to show")
	}
	if err := exec.Command("xdg-open", filepath.Dir(filepath.Clean(path))).Start(); err != nil {
		return fmt.Errorf("app: opening the file manager: %w", err)
	}
	return nil
}
