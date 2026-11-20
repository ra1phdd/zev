package childprocess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	helperEnv  = "ZEV_CHILD_PROCESS_HELPER"
	roleEnv    = "ZEV_CHILD_PROCESS_ROLE"
	readyEnv   = "ZEV_CHILD_PROCESS_READY"
	parentRole = "parent"
	childRole  = "child"
	exitRole   = "exit"
)

func TestRunPreservesSuccessfulExit(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^NoChildProcessTest$") //nolint:gosec // Fixed selector in the current test binary.
	if err := Run(t.Context(), cmd); err != nil {
		t.Fatalf("Run() = %v, want successful exit", err)
	}
}

func TestRunPreservesChildExitError(t *testing.T) {
	t.Parallel()
	cmd := helperCommand(t, exitRole)
	err := Run(t.Context(), cmd)
	if err == nil {
		t.Fatal("Run() error = nil, want child exit error")
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %T, want *exec.ExitError", err)
	}
	if got := exitErr.ExitCode(); got != 23 {
		t.Errorf("ExitCode() = %d, want 23", got)
	}
}

func TestRunRejectsAlreadyCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := Run(ctx, helperCommand(t, exitRole))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestChildProcessHelper(t *testing.T) {
	t.Parallel()
	t.Helper()
	if os.Getenv(helperEnv) != "1" {
		return
	}

	switch os.Getenv(roleEnv) {
	case exitRole:
		os.Exit(23)
	case childRole:
		blockProcess()
	case parentRole:
		runParentHelper(t)
	default:
		t.Fatalf("unknown helper role %q", os.Getenv(roleEnv))
	}
}

func helperCommand(t *testing.T, role string) *exec.Cmd {
	t.Helper()
	return helperCommandContext(t.Context(), t, role, "")
}

func helperCommandContext(ctx context.Context, t *testing.T, role, readyPath string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestChildProcessHelper$", "-test.v") //nolint:gosec // Test launches the current test binary with a fixed test selector.
	cmd.Env = append(os.Environ(),
		helperEnv+"=1",
		roleEnv+"="+role,
		readyEnv+"="+readyPath,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd
}

func runParentHelper(t *testing.T) {
	t.Helper()
	readyPath := os.Getenv(readyEnv)
	if readyPath == "" {
		t.Fatal("missing readiness path")
	}

	child := helperCommand(t, childRole)
	if err := child.Start(); err != nil {
		t.Fatalf("start helper child: %v", err)
	}
	if err := publishReady(readyPath, fmt.Appendf(nil, "parent=%d\nchild=%d\n", os.Getpid(), child.Process.Pid)); err != nil {
		if killErr := child.Process.Kill(); killErr != nil {
			t.Logf("kill helper child: %v", killErr)
		}
		if waitErr := child.Wait(); waitErr != nil {
			t.Logf("wait helper child: %v", waitErr)
		}
		t.Fatalf("write readiness: %v", err)
	}
	blockProcess()
}

func publishReady(path string, data []byte) error {
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil { //nolint:gosec // Test-only path is supplied by this test process through an environment variable.
		return fmt.Errorf("write readiness: %w", err)
	}
	if err := os.Rename(path+".tmp", path); err != nil { //nolint:gosec // Test-only path is supplied by this test process through an environment variable.
		return fmt.Errorf("publish readiness: %w", err)
	}
	return nil
}

func blockProcess() {
	for {
		time.Sleep(time.Hour)
	}
}

func readReady(t *testing.T, path string) (parentPID, childPID int) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path) //nolint:gosec // The path is created under t.TempDir and is used only by this test.
		if err == nil {
			values := make(map[string]int)
			for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					t.Fatalf("invalid readiness line %q", line)
				}
				value, parseErr := strconv.Atoi(parts[1])
				if parseErr != nil {
					t.Fatalf("parse readiness line %q: %v", line, parseErr)
				}
				values[parts[0]] = value
			}
			return values["parent"], values["child"]
		}

		select {
		case <-deadline.C:
			t.Fatalf("waiting for readiness file %q: %v", path, err)
		case <-ticker.C:
		}
	}
}
