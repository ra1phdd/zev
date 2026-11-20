//go:build !windows

package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openReadFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0) //nolint:gosec // Caller selects the trusted parent; O_NOFOLLOW protects the leaf.
	if err != nil {
		return nil, err // Preserve os.IsNotExist compatibility for missing files.
	}
	return file, nil
}

func openLockFile(path string) (_ *os.File, err error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open nofollow sidecar: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, fmt.Errorf("stat sidecar: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return nil, errors.New("lock sidecar must be a singly linked regular file")
	}
	if err := validateLockIdentity(file, path); err != nil {
		return nil, err
	}
	return file, nil
}

func lockFile(ctx context.Context, file *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait for flock: %w", err)
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			return fmt.Errorf("flock: %w", err)
		}
		if err := waitLockRetry(ctx); err != nil {
			return err
		}
	}
}

func syncDirectory(path string) (err error) {
	file, err := os.Open(path) //nolint:gosec // Directory belongs to the caller-selected storage location.
	if err != nil {
		return fmt.Errorf("open storage directory: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close storage directory: %w", closeErr))
		}
	}()
	if err := file.Sync(); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOTSUP) {
		return fmt.Errorf("sync storage directory: %w", err)
	}
	return nil
}
