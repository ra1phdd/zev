//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package childprocess

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func waitForExit(pid int) (err error) {
	fd, err := unix.Kqueue()
	if err != nil {
		return fmt.Errorf("create process exit observer: %w", err)
	}
	defer func() { err = errors.Join(err, unix.Close(fd)) }()
	unix.CloseOnExec(fd)
	change := unix.Kevent_t{}
	unix.SetKevent(&change, pid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT)
	change.Fflags = unix.NOTE_EXIT
	changes := []unix.Kevent_t{change}
	// An unreaped child can already be a zombie when registration runs.
	if _, err := unix.Kevent(fd, changes, nil, nil); err != nil {
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		return fmt.Errorf("observe process exit: %w", err)
	}
	events := make([]unix.Kevent_t, 1)
	for {
		n, err := unix.Kevent(fd, nil, events, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("wait for process exit: %w", err)
		}
		if n > 0 {
			return nil
		}
	}
}
