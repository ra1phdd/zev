// Package terminal executes shell commands and injects text into terminal input.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/ra1phdd/zev/internal/core/childprocess"
)

var (
	// ErrNotTTY indicates that standard input is not a terminal.
	ErrNotTTY = errors.New("stdin is not a TTY")

	// ErrInjectionFailed indicates that the TIOCSTI ioctl failed.
	ErrInjectionFailed = errors.New("failed to inject command")

	// ErrInjectionUnsupported indicates that terminal injection is unavailable.
	ErrInjectionUnsupported = errors.New("command injection is not supported on this platform")
)

// Execute runs command as shell code using SHELL, or sh when SHELL is unset,
// with the current process's standard streams. The caller must authorize execution
// of command; this function does not sanitize shell syntax or prompt for confirmation.
// It returns a wrapped process error on failure, preserving errors.As for
// *exec.ExitError and errors.Is for context cancellation before launch.
func Execute(ctx context.Context, command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}

	cmd := exec.CommandContext(ctx, shell, "-c", command) //nolint:gosec // This authorized shell-execution boundary intentionally accepts shell syntax and the user's SHELL.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := childprocess.Run(ctx, cmd); err != nil {
		return fmt.Errorf("execute shell command: %w", err)
	}
	return nil
}
