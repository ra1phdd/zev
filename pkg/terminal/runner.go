package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/ra1phdd/zev/internal/core/childprocess"
	"github.com/ra1phdd/zev/internal/core/security"
)

// Runner executes either shell text or literal argv through one process boundary.
// It owns the child process and forwards the supplied standard streams.
type Runner struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Dir    string
	Env    []string
}

// RunShell runs command through shell -c using the configured streams.
func (r Runner) RunShell(ctx context.Context, shell, command string) error {
	if shell == "" {
		shell = "sh"
	}
	// Shell text is an explicit caller-selected execution mode.
	return r.run(ctx, exec.CommandContext(ctx, shell, "-c", command)) //nolint:gosec // The shell is part of the validated delivery contract.
}

// RunArgv runs executable with literal args using the configured streams.
// Arguments are passed unchanged and are never joined or reparsed by a shell.
func (r Runner) RunArgv(ctx context.Context, executable string, args []string) error {
	if executable == "" {
		return errors.New("run argv: executable is empty")
	}
	return r.run(ctx, exec.CommandContext(ctx, executable, args...)) //nolint:gosec // Literal argv execution is the runner's purpose.
}

func (r Runner) run(ctx context.Context, cmd *exec.Cmd) error {
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	cmd.Dir = r.Dir
	if r.Env == nil {
		cmd.Env = security.SanitizedEnvironment(os.Environ())
	} else {
		cmd.Env = append([]string(nil), r.Env...)
	}
	if err := childprocess.Run(ctx, cmd); err != nil {
		return fmt.Errorf("run child process: %w", err)
	}
	return nil
}

// DefaultRunner returns a proxy runner connected to the current process streams.
func DefaultRunner() Runner {
	return Runner{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
}
