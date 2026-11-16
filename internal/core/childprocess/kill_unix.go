//go:build !windows && !darwin

package childprocess

import (
	"fmt"
	"syscall"
)

func killGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("signal owned process group: %w", err)
	}
	return nil
}
