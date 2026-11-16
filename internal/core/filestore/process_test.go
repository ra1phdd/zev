package filestore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWithLockAcrossProcesses(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	const processes, updates = 4, 10
	commands := make([]*exec.Cmd, 0, processes)
	for range processes {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestFileLockProcessHelper$") //nolint:gosec // Runs this test executable, obtained from os.Executable.
		cmd.Env = append(os.Environ(), "ZEV_FILESTORE_HELPER_PATH="+path)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Errorf("lock worker: %v", err)
		}
	}
	data, err := os.ReadFile(path) //nolint:gosec // Test path is created in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != processes*updates {
		t.Errorf("got %d updates, want %d", len(data), processes*updates)
	}
}

func TestFileLockProcessHelper(t *testing.T) {
	t.Parallel()
	path := os.Getenv("ZEV_FILESTORE_HELPER_PATH")
	if path == "" {
		return
	}
	for range 10 {
		if err := WithLock(path, func() error {
			data, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // Test path is supplied through the test helper environment.
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("read shared data: %w", err)
			}
			time.Sleep(time.Millisecond)
			return AtomicWrite(path, append(data, 'x'), 0o600)
		}); err != nil {
			t.Fatal(err)
		}
	}
}
