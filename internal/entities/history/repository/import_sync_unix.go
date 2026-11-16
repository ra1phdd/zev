//go:build darwin || linux

package repository

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func syncArchiveDirectory(path string) (err error) {
	file, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open archive directory: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync archive directory: %w", err)
	}
	return nil
}
