//go:build !windows

package childprocess

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func prepareCommand(cmd *exec.Cmd) (processControl, error) {
	restore, err := prepareTerminal(cmd)
	if err != nil {
		return processControl{}, err
	}
	return processControl{
		started:    func() error { return nil },
		waitExited: func() error { return waitForExit(cmd.Process.Pid) },
		kill:       func() error { return killProcess(cmd) },
		close:      restore,
	}, nil
}

func prepareTerminal(cmd *exec.Cmd) (func() error, error) {
	attr := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attr = *cmd.SysProcAttr
	}
	attr.Setpgid = true
	attr.Pgid = 0
	cmd.SysProcAttr = &attr

	restore := func() error { return nil }
	stdin, ok := cmd.Stdin.(*os.File)
	if !ok || !term.IsTerminal(int(stdin.Fd())) {
		return restore, nil
	}
	fd := int(stdin.Fd())
	group, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return nil, fmt.Errorf("read terminal foreground group: %w", err)
	}
	if group != syscall.Getpgrp() {
		return restore, nil
	}

	// A separate group needs foreground ownership for interactive terminal reads.
	attr.Foreground = true
	attr.Ctty = fd
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTTOU)
	return func() error {
		defer signal.Stop(signals)
		if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, group); err != nil {
			return fmt.Errorf("restore terminal foreground group: %w", err)
		}
		return nil
	}, nil
}

func killProcess(cmd *exec.Cmd) error {
	groupErr := killGroup(cmd.Process.Pid)
	processErr := cmd.Process.Kill()
	if errors.Is(groupErr, syscall.ESRCH) {
		groupErr = nil
	}
	if errors.Is(processErr, os.ErrProcessDone) {
		processErr = nil
	}
	if groupErr != nil {
		groupErr = fmt.Errorf("kill owned process group: %w", groupErr)
	}
	if processErr != nil {
		processErr = fmt.Errorf("kill owned leader: %w", processErr)
	}
	return errors.Join(groupErr, processErr)
}
