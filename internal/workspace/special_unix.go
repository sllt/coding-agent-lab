//go:build unix

package workspace

import (
	"fmt"
	"os"
	"syscall"
)

func syscallMknod(path string) error {
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		return fmt.Errorf("mkfifo: %w", err)
	}
	if _, err := os.Lstat(path); err != nil {
		return err
	}
	return nil
}
