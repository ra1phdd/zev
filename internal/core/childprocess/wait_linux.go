//go:build linux

package childprocess

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func waitForExit(pid int) error {
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("observe process exit without reaping: %w", err)
		}
		return nil
	}
}
