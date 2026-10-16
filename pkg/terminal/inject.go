package terminal

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

var (
	// ErrNotTTY возвращается, когда stdin не является TTY.
	ErrNotTTY = errors.New("stdin is not a TTY")

	// ErrInjectionFailed возвращается, когда TIOCSTI ioctl не сработал.
	ErrInjectionFailed = errors.New("failed to inject command")

	// ErrInjectionUnsupported возвращается на платформах без поддержки TIOCSTI.
	ErrInjectionUnsupported = errors.New("command injection is not supported on this platform")
)

// Execute выполняет команду через shell напрямую.
// Подключает stdin/stdout/stderr к текущему процессу.
// Возвращает ошибку, если команда завершилась с ненулевым кодом.
func Execute(ctx context.Context, command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}

	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
