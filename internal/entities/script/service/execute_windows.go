//go:build windows

package service

import (
	"context"
	"errors"
	"os"
)

func runScript(_ context.Context, _ *os.File, _ []string) error {
	return errors.New("script execution is unsupported on Windows without a verified handle-safe Bash transport")
}
