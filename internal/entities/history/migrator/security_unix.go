//go:build !windows

package migrator

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func secureLegacyPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect legacy path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: legacy file is not regular", service.ErrInvalid)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure legacy file: %w", err)
	}
	secured, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify legacy permissions: %w", err)
	}
	if !secured.Mode().IsRegular() || !os.SameFile(info, secured) || secured.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: legacy file permissions or identity changed", service.ErrInvalid)
	}
	return nil
}
