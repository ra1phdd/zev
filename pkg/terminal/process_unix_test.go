//go:build !windows

package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestExecuteCancellationKillsShellDescendants(t *testing.T) {
	t.Parallel()

	readyPath := filepath.Join(t.TempDir(), "child-pid")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	command := "sleep 60 & printf '%s' $! > '" + readyPath + ".tmp'; mv '" + readyPath + ".tmp' '" + readyPath + "'; wait"

	done := make(chan error, 1)
	go func() { done <- Execute(ctx, command) }()
	childPID := waitForPID(t, readyPath)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Execute() did not return after cancellation")
	}
	waitForProcessGone(t, childPID)
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path) //nolint:gosec // The path is created under t.TempDir and is used only by this test.
		if err == nil && len(data) > 0 {
			pid, parseErr := strconv.Atoi(string(data))
			if parseErr != nil {
				t.Fatalf("parse child PID: %v", parseErr)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child PID was not written to %q", path)
	return 0
}

func waitForProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d is still alive", pid)
}
