package filetxn

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func moveExclusive(from, to string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("rename configuration: %w", err)
	}
	return nil
}
