//go:build windows

package childprocess

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestRunCancellationKillsWindowsJobDescendants(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := helperCommandContext(ctx, t, parentRole, readyPath)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cmd) }()
	parentPID, childPID := readReady(t, readyPath)
	// Retain handles before cancellation; observations do not rely on PID reuse.
	parent := openProcessForWait(t, parentPID)
	child := openProcessForWait(t, childPID)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not join job cancellation")
	}
	for _, process := range []windows.Handle{parent, child} {
		status, err := windows.WaitForSingleObject(process, 5000)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			t.Errorf("owned process wait = %d, %v", status, err)
		}
	}
}

func openProcessForWait(t *testing.T, pid int) windows.Handle {
	t.Helper()
	pid32, err := windowsProcessID(pid)
	if err != nil {
		t.Fatal(err)
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid32)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := windows.CloseHandle(process); err != nil {
			t.Errorf("close process observation: %v", err)
		}
	})
	return process
}
