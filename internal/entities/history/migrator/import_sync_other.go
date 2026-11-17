//go:build !darwin && !linux && !windows

package migrator

import "errors"

func syncArchiveDirectory(string) error {
	return errors.New("archive directory synchronization is unsupported")
}
