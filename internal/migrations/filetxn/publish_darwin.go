package filetxn

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func moveExclusive(from, to string) error {
	if err := unix.RenamexNp(from, to, unix.RENAME_EXCL); err != nil {
		return fmt.Errorf("rename configuration: %w", err)
	}
	return nil
}
