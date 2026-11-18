package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/ra1phdd/zev/internal/core/childprocess"
)

// ExecuteRendered launches a validated argv-safe alias through an allowlisted
// POSIX shell. RenderAlias must have validated and rendered command first.
// Child errors preserve *exec.ExitError for exit-status propagation.
func ExecuteRendered(ctx context.Context, command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	base := filepath.Base(shell)
	switch base {
	case "sh", "bash", "dash", "ash", "ksh", "zsh":
	default:
		return fmt.Errorf("unsupported alias shell %q: a POSIX-compatible shell is required", shell)
	}
	if runtime.GOOS == "windows" {
		return errors.New("argv-safe alias execution is unsupported on Windows without verified POSIX argv transport")
	}
	cmd := exec.CommandContext(ctx, shell, "-c", command) //nolint:gosec // Only validated argv-safe aliases enter this allowlisted shell boundary.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := childprocess.Run(ctx, cmd); err != nil {
		return fmt.Errorf("execute alias: %w", err)
	}
	return nil
}
