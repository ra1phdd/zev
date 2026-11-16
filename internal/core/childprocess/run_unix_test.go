//go:build !windows

package childprocess

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestRunOwnsCommandContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := helperCommandContext(ctx, t, parentRole, readyPath)
	var directCancel atomic.Bool
	cmd.Cancel = func() error {
		directCancel.Store(true)
		return cmd.Process.Kill()
	}
	result := make(chan error, 1)
	go func() { result <- Run(ctx, cmd) }()
	parentPID, childPID := readReady(t, readyPath)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() = %v, want cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not join cancellation")
	}
	assertProcessesGone(t, parentPID, childPID)
	if directCancel.Load() {
		t.Fatal("CommandContext killed the leader outside process-group coordination")
	}
}

func TestRunCleansDescendantsBeforeReapingExitedLeader(t *testing.T) {
	t.Parallel()
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `sleep 60 & printf '%s' "$!" > "$1.tmp"; mv "$1.tmp" "$1"`, "sh", readyPath) //nolint:gosec // Fixed shell fixture with a test-owned readiness path.
	cmd.Stdout = new(bytes.Buffer)
	cmd.WaitDelay = 100 * time.Millisecond
	if err := Run(t.Context(), cmd); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	data, err := os.ReadFile(readyPath) //nolint:gosec // Test-owned readiness path.
	if err != nil {
		t.Fatal(err)
	}
	var childPID int
	if _, err := fmt.Sscan(string(data), &childPID); err != nil {
		t.Fatal(err)
	}
	assertProcessesGone(t, childPID)
}

func TestRunCancellationKillsProcessTree(t *testing.T) {
	t.Parallel()
	parentPID, childPID, err := runCancelledTree(t, context.WithCancel, true)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	assertExitError(t, err)
	assertProcessesGone(t, parentPID, childPID)
}

func TestRunDeadlineKillsProcessTree(t *testing.T) {
	t.Parallel()
	parentPID, childPID, err := runCancelledTree(t, func(parent context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(parent, 100*time.Millisecond)
	}, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	assertExitError(t, err)
	assertProcessesGone(t, parentPID, childPID)
}

func runCancelledTree(t *testing.T, makeContext func(context.Context) (context.Context, context.CancelFunc), cancelAfterReady bool) (int, int, error) {
	t.Helper()
	readyPath := filepath.Join(t.TempDir(), "ready")
	ctx, cancel := makeContext(t.Context())
	defer cancel()
	cmd := helperCommandContext(ctx, t, parentRole, readyPath)

	result := make(chan error, 1)
	go func() { result <- Run(ctx, cmd) }()
	parentPID, childPID := readReady(t, readyPath)
	if cancelAfterReady {
		cancel()
	}

	select {
	case err := <-result:
		return parentPID, childPID, err
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancellation")
		return 0, 0, nil
	}
}

func assertExitError(t *testing.T, err error) {
	t.Helper()
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) {
		t.Errorf("Run() error = %T, want an error with ExitCode", err)
	}
}

func assertProcessesGone(t *testing.T, pids ...int) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !slices.ContainsFunc(pids, processExists) {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("processes still alive: %v", pids)
		case <-ticker.C:
		}
	}
}

func processExists(pid int) bool {
	if err := syscall.Kill(pid, 0); err == nil {
		return true
	} else if errors.Is(err, syscall.ESRCH) {
		return false
	}
	return true
}
