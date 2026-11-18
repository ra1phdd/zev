//go:build !darwin && !linux && !windows

package history

import (
	"errors"
	"os"
)

func openLegacySource(path string) (*os.File, error) {
	if _, err := os.Lstat(path); err != nil {
		return nil, err
	}
	return nil, errors.New("safe legacy file opening is unsupported on this platform")
}

func secureLegacyFile(*os.File) error {
	return errors.New("legacy file security repair is unsupported on this platform")
}

func validateLegacyFileInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("legacy file must be regular")
	}
	return nil
}
