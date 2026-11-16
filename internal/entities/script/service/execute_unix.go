//go:build !windows

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/ra1phdd/zev/internal/core/childprocess"
)

func runScript(ctx context.Context, file *os.File, args []string) error {
	argv := append([]string{"/dev/fd/3"}, args...)
	cmd := exec.CommandContext(ctx, "bash", argv...) //nolint:gosec // Fixed interpreter receives an inherited, pinned script descriptor and literal argv.
	cmd.ExtraFiles = []*os.File{file}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := childprocess.Run(ctx, cmd); err != nil {
		return fmt.Errorf("run pinned script: %w", err)
	}
	return nil
}
