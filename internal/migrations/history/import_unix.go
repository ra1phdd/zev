//go:build darwin || linux

package history

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/ra1phdd/zev/internal/entities/history/service"
	"golang.org/x/sys/unix"
)

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

func secureLegacyFile(file *os.File) error {
	before, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened legacy file: %w", err)
	}
	if err := validateLegacyFileInfo(before); err != nil {
		return err
	}
	if err := unix.Fchmod(int(file.Fd()), 0o600); err != nil {
		return fmt.Errorf("secure legacy file: %w", err)
	}
	secured, err := file.Stat()
	if err != nil {
		return fmt.Errorf("verify legacy permissions: %w", err)
	}
	if err := validateLegacyFileInfo(secured); err != nil {
		return err
	}
	if !os.SameFile(before, secured) || secured.Mode().Perm() != 0o600 {
		return fmt.Errorf("%w: legacy file permissions or identity changed", service.ErrInvalid)
	}
	return nil
}

func validateLegacyFileInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: legacy file is not regular", service.ErrInvalid)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return fmt.Errorf("%w: legacy file must have exactly one hard link", service.ErrInvalid)
	}
	return nil
}
