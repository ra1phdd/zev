//go:build !darwin && !linux && !windows

package migrator

import (
	"errors"
	"os"
)

func secureLegacyFile(*os.File) error {
	return errors.New("legacy file security repair is unsupported on this platform")
}

func validateLegacyFileInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("legacy file must be regular")
	}
	return nil
}
