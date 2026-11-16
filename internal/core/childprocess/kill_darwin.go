//go:build darwin

package childprocess

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func killGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("signal owned process group: %w", err)
	}
	members, queryErr := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pid)
	return classifyGroupSignal(pid, err, members, queryErr)
}

func classifyGroupSignal(pid int, signalErr error, members []unix.KinfoProc, queryErr error) error {
	if queryErr != nil {
		return errors.Join(signalErr, fmt.Errorf("inspect owned process group: %w", queryErr))
	}
	// XNU killpg1 excludes SZOMB from its iteration and returns EPERM when no
	// signalable members remain. Suppress only a confirmed zombie-only group
	// containing our unreaped leader; a permission error on a live group is real.
	const zombie = 5 // SZOMB in Darwin sys/proc.h.
	leaderFound := false
	for _, member := range members {
		if member.Proc.P_stat != zombie {
			return signalErr
		}
		if int(member.Proc.P_pid) == pid {
			leaderFound = true
		}
	}
	if !leaderFound {
		return signalErr
	}
	return nil
}
