//go:build !windows

package filetxn

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openFile(path string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Clean(path), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open nofollow: %w", err)
	}
	return f, nil
}

func identity(file *os.File) (string, error) {
	var st unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &st); err != nil {
		return "", fmt.Errorf("stat config file: %w", err)
	}
	if st.Nlink != 1 {
		return "", errors.New("config file must have one hard link")
	}
	return fmt.Sprintf("%x:%x", st.Dev, st.Ino), nil
}

// SyncDirectory flushes migration directory entries and reports sync failures.
func SyncDirectory(path string) (err error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("open migration directory: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync migration directory: %w", err)
	}
	return nil
}
