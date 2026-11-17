//go:build !linux && !darwin && !windows

package migrator

import (
	"errors"
	"os"
)

func renameExclusive(string, string) error {
	return errors.New("exclusive archive rename is unsupported on this platform")
}

func openLegacySource(path string) (*os.File, error) {
	if _, err := os.Lstat(path); err != nil {
		return nil, err
	}
	return nil, errors.New("safe legacy file opening is unsupported on this platform")
}
