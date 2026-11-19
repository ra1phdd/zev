package terminal

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
)

func TestRunnerRunArgvForwardsStreamsAndPreservesArgumentBoundaries(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	runner := Runner{Stdout: &stdout, Stderr: &stderr}

	err := runner.RunArgv(t.Context(), "/bin/sh", []string{"-c", "printf '%s' \"$1\"; printf '%s' \"$2\" >&2", "sh", "a b", "c;d"})
	if err != nil {
		t.Fatalf("RunArgv() error = %v", err)
	}
	if got, want := stdout.String(), "a b"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "c;d"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunnerRunShellUsesSameProxyBoundary(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	runner := Runner{Stdout: &stdout, Stderr: &stderr}

	if err := runner.RunShell(t.Context(), "/bin/sh", "printf out; printf err >&2"); err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if stdout.String() != "out" || stderr.String() != "err" {
		t.Fatalf("streams = stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func TestRunnerPreservesChildExitError(t *testing.T) {
	t.Parallel()
	runner := Runner{Stdout: bytes.NewBuffer(nil), Stderr: bytes.NewBuffer(nil)}

	err := runner.RunArgv(t.Context(), "/bin/sh", []string{"-c", "exit 7"})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("RunArgv() error = %v, want exit code 7", err)
	}
}
