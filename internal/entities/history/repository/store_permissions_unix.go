//go:build !windows

package repository

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func secureHistoryPath(path string, isDir bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect history path: %w", err)
	}
	if isDir {
		if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("%w: history directory must be private (0700)", service.ErrInvalid)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: history file is not regular", service.ErrInvalid)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure history file: %w", err)
	}
	secured, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify history permissions: %w", err)
	}
	if !secured.Mode().IsRegular() || !os.SameFile(info, secured) || secured.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: history file permissions or identity changed", service.ErrInvalid)
	}
	return nil
}
