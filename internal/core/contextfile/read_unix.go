//go:build !windows

package contextfile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openContextLeaf(root *os.Root, name string) (file *os.File, err error) {
	parent, err := root.OpenFile(".", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open pinned context parent: %w", err)
	}
	defer func() {
		if closeErr := parent.Close(); closeErr != nil {
			if file != nil {
				closeErr = errors.Join(closeErr, file.Close())
				file = nil
			}
			err = errors.Join(err, fmt.Errorf("close context parent: %w", closeErr))
		}
	}()
	// Root.OpenFile resolves confined symlinks even with O_NOFOLLOW. Open the
	// single leaf directly against the pinned parent to retain kernel nofollow
	// and nonblocking semantics during regular-file/FIFO replacement races.
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			err = errors.Join(ErrSymlink, err)
		}
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	return os.NewFile(uintptr(fd), name), nil
}
