//go:build windows

package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/ra1phdd/zev/internal/core/childprocess"
)

func runScript(ctx context.Context, file *os.File, args []string) error {
	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read pinned script: %w", err)
	}
	argv := append([]string{"-c", string(data), file.Name()}, args...)
	cmd := exec.CommandContext(ctx, "bash", argv...) //nolint:gosec // Source is read from the pinned script; arguments remain separate argv values.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := childprocess.Run(ctx, cmd); err != nil {
		return fmt.Errorf("run pinned script: %w", err)
	}
	return nil
}
