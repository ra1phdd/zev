//go:build !windows

package filestore

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func checkSingleLink(f *os.File) error {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return fmt.Errorf("inspect storage file links: %w", err)
	}
	if st.Nlink != 1 {
		return errors.New("storage file must have one hard link")
	}
	return nil
}
