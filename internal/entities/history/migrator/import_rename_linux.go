//go:build linux

package migrator

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func renameExclusive(oldPath, newPath string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("rename archive exclusively: %w", err)
	}
	return nil
}

func openLegacySource(path string) (*os.File, error) {
	named, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect legacy source: %w", err)
	}
	if err := validateLegacyFileInfo(named); err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open legacy source without following links: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	opened, statErr := file.Stat()
	if statErr != nil {
		return nil, errors.Join(fmt.Errorf("stat legacy source: %w", statErr), file.Close())
	}
	if !os.SameFile(named, opened) {
		return nil, errors.Join(errors.New("legacy source changed during open"), file.Close())
	}
	if err := validateLegacyFileInfo(opened); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}
