// Package terminal executes shell commands and injects text into terminal input.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
)

var (
	// ErrNotTTY indicates that standard input is not a terminal.
	ErrNotTTY = errors.New("stdin is not a TTY")

	// ErrInjectionFailed indicates that the TIOCSTI ioctl failed.
	ErrInjectionFailed = errors.New("failed to inject command")

	// ErrInjectionUnsupported indicates that terminal injection is unavailable.
	ErrInjectionUnsupported = errors.New("command injection is not supported on this platform")
)

// PartialInjectionError reports that terminal input may contain a prefix of the
// requested text. Callers must not report a printed fallback as successful delivery.
type PartialInjectionError struct {
	Written int
	Err     error
}

func (e *PartialInjectionError) Error() string {
	return fmt.Sprintf("terminal injection wrote %d bytes: %v", e.Written, e.Err)
}

func (e *PartialInjectionError) Unwrap() error { return e.Err }

// Execute runs command as shell code using SHELL, or sh when SHELL is unset,
// with the current process's standard streams. The caller must authorize execution
// of command; this function does not sanitize shell syntax or prompt for confirmation.
func Execute(ctx context.Context, command string) error {
	shell := shellFromEnvironment()
	return DefaultRunner().RunShell(ctx, shell, command)
}

func shellFromEnvironment() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	return shell
}

// DefaultShell returns the configured shell, falling back to sh.
func DefaultShell() string {
	return shellFromEnvironment()
}
