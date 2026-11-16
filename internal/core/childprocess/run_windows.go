//go:build windows

package childprocess

import (
	"errors"
	"fmt"
	"math"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareCommand(cmd *exec.Cmd) (processControl, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processControl{}, fmt.Errorf("create process job: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return processControl{}, errors.Join(fmt.Errorf("configure process job: %w", err), windows.CloseHandle(job))
	}
	attr := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attr = *cmd.SysProcAttr
	}
	// os/exec closes the primary thread handle. Resume it through a thread
	// snapshot only after assignment, preventing a pre-assignment spawn race.
	attr.CreationFlags |= windows.CREATE_SUSPENDED
	cmd.SysProcAttr = &attr
	var process windows.Handle
	return processControl{
		started: func() error {
			pid, err := windowsProcessID(cmd.Process.Pid)
			if err != nil {
				return err
			}
			process, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
			if err != nil {
				return fmt.Errorf("open owned process: %w", err)
			}
			if err := windows.AssignProcessToJobObject(job, process); err != nil {
				return fmt.Errorf("assign process job: %w", err)
			}
			return resumeOwnedProcess(pid)
		},
		waitExited: func() error {
			if _, err := windows.WaitForSingleObject(process, windows.INFINITE); err != nil {
				return fmt.Errorf("observe process exit: %w", err)
			}
			return nil
		},
		kill: func() error {
			if err := windows.TerminateJobObject(job, 1); err != nil {
				return fmt.Errorf("terminate process job: %w", err)
			}
			return nil
		},
		close: func() error {
			jobErr := windows.CloseHandle(job)
			var processErr error
			if process != 0 {
				processErr = windows.CloseHandle(process)
			}
			return errors.Join(jobErr, processErr)
		},
	}, nil
}

func windowsProcessID(pid int) (uint32, error) {
	value := int64(pid)
	if value <= 0 || value > math.MaxUint32 {
		return 0, fmt.Errorf("invalid owned process ID: %d", pid)
	}
	return uint32(value), nil
}

func resumeOwnedProcess(pid uint32) (err error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot suspended process thread: %w", err)
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(snapshot)) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	nextErr := windows.Thread32First(snapshot, &entry)
	for ; nextErr == nil; nextErr = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("open suspended process thread: %w", err)
		}
		previous, resumeErr := windows.ResumeThread(thread)
		closeErr := windows.CloseHandle(thread)
		if resumeErr != nil {
			return errors.Join(fmt.Errorf("resume owned process: %w", resumeErr), closeErr)
		}
		if previous != 1 {
			return errors.Join(fmt.Errorf("unexpected process thread suspend count: %d", previous), closeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close resumed process thread: %w", closeErr)
		}
		return nil
	}
	if !errors.Is(nextErr, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate suspended process threads: %w", nextErr)
	}
	return fmt.Errorf("find suspended thread for owned process %d: %w", pid, windows.ERROR_NOT_FOUND)
}
