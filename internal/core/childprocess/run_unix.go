//go:build !windows

package childprocess

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func prepareCommand(cmd *exec.Cmd) (processControl, error) {
	attr := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attr = *cmd.SysProcAttr
	}
	attr.Setpgid = true
	attr.Pgid = 0
	cmd.SysProcAttr = &attr
	return processControl{
		started:    func() error { return nil },
		waitExited: func() error { return waitForExit(cmd.Process.Pid) },
		kill:       func() error { return killProcess(cmd) },
		restore:    func() error { return nil },
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
