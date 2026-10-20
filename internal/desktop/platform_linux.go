//go:build linux

package desktop

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

func shellOpen(path, arguments string) error {
	if arguments != "" {
		return fmt.Errorf("Linux open does not accept shell arguments")
	}
	// Pass a single absolute path directly, never through a shell.
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cmd := exec.Command("xdg-open", absolute)
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("xdg-open: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
func revealPath(path string) error { return shellOpen(filepath.Dir(path), "") }
