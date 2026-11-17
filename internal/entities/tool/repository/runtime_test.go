package repository

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	helperSelector = "-test.run=^TestExecHelper$"
	controlMode    = "controls"
	exitMode       = "exit"
	outputMode     = "output"
	sleepMode      = "sleep"
)

func TestApprovedExecution(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	request := ExecRequest{Command: os.Args[0], Args: []string{helperSelector, "--", "literal", ";$(touch marker)", "*", "|"}}
	result := e.Execute(t.Context(), request, false)
	if result.Denied || result.ExitCode != 0 || !strings.Contains(result.Output, "literal=;$(touch marker),*,|") || !strings.Contains(result.Output, "cwd="+root) {
		t.Fatalf("literal argv child: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "marker")); !os.IsNotExist(err) {
		t.Fatal("shell syntax was evaluated")
	}
}

func TestApprovedEnvironmentIsolation(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "provider-secret-must-not-inherit")
	t.Setenv("PATH", t.TempDir())
	e, _ := newExecutor(t)
	result := e.Execute(t.Context(), ExecRequest{Command: os.Args[0], Args: []string{helperSelector, "--", "environment"}}, false)
	expectedPath := "/usr/bin:/bin"
	if runtime.GOOS == windowsOS {
		expectedPath = filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	if result.Denied || result.ExitCode != 0 || strings.Contains(result.Output, "provider-secret") || !strings.Contains(result.Output, "key=,path="+expectedPath) {
		t.Fatalf("unsafe child environment: %+v", result)
	}
}

func TestApprovedOutputAndExit(t *testing.T) {
	t.Parallel()
	e, _ := newExecutor(t)
	for _, mode := range []string{outputMode, controlMode, exitMode} {
		result := e.Execute(t.Context(), ExecRequest{Command: os.Args[0], Args: []string{helperSelector, "--", mode}}, false)
		if result.Denied {
			t.Fatal(result)
		}
		switch mode {
		case outputMode:
			if len(result.Output) > maxOutput || !result.Truncated || !strings.Contains(result.Output, "stderr") {
				t.Fatalf("output cap: %+v", result)
			}
		case controlMode:
			if strings.ContainsAny(result.Output, "\x1b\r\u202e\x00") || !strings.Contains(result.Output, `\x1b`) {
				t.Fatalf("unsafe controls: %+v", result)
			}
		case exitMode:
			if result.ExitCode != 23 {
				t.Fatalf("exit status: %+v", result)
			}
		}
	}
}

func TestApprovedCancellation(t *testing.T) {
	t.Parallel()
	e, _ := newExecutor(t)
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	t.Cleanup(cancel)
	start := time.Now()
	result := e.Execute(ctx, ExecRequest{Command: os.Args[0], Args: []string{helperSelector, "--", sleepMode}}, false)
	if result.ExitCode != 124 || result.Denied || time.Since(start) > 3*time.Second {
		t.Fatalf("deadline not enforced: %+v, duration=%v", result, time.Since(start))
	}
	canceled, cancelNow := context.WithCancel(t.Context())
	cancelNow()
	result = e.Execute(canceled, ExecRequest{Command: builtinPWD}, true)
	if result.ExitCode != 130 {
		t.Fatalf("cancellation: %+v", result)
	}
}

func TestApprovedDefaultDeadline(t *testing.T) {
	t.Parallel()
	e, _ := newExecutor(t)
	start := time.Now()
	result := e.Execute(t.Context(), ExecRequest{Command: os.Args[0], Args: []string{helperSelector, "--", sleepMode}}, false)
	duration := time.Since(start)
	if result.ExitCode != 124 || result.Denied || duration < 9*time.Second || duration > 13*time.Second {
		t.Fatalf("default deadline result=%+v duration=%v", result, duration)
	}
}

func TestApprovedExplicitShellAndNoStdin(t *testing.T) {
	t.Parallel()
	e, _ := newExecutor(t)
	result := e.Execute(t.Context(), ExecRequest{Command: os.Args[0], Args: []string{helperSelector, "--", "stdin"}}, false)
	if result.ExitCode != 0 || result.Denied {
		t.Fatalf("stdin was not closed: %+v", result)
	}
	if runtime.GOOS == windowsOS {
		return
	}
	result = e.Execute(t.Context(), ExecRequest{Command: "sh", Args: []string{"-c", "printf approved"}}, false)
	if result.ExitCode != 0 || result.Output != "approved" {
		t.Fatalf("explicit approved shell failed: %+v", result)
	}
	result = e.Execute(t.Context(), ExecRequest{Command: "not-a-real-tool-executable"}, false)
	if result.ExitCode != -1 || result.Denied || result.Reason != "process execution failed" {
		t.Fatalf("missing executable: %+v", result)
	}
}

func TestExecHelper(t *testing.T) {
	t.Parallel()
	index := -1
	for i, arg := range os.Args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index < 0 || index+1 >= len(os.Args) {
		return
	}
	args := os.Args[index+1:]
	switch args[0] {
	case "literal":
		cwd, err := os.Getwd()
		if err != nil {
			os.Exit(24)
		}
		if _, err := fmt.Fprintf(os.Stdout, "literal=%s\ncwd=%s\n", strings.Join(args[1:], ","), cwd); err != nil {
			os.Exit(25)
		}
	case "environment":
		if _, err := fmt.Fprintf(os.Stdout, "key=%s,path=%s", os.Getenv("OPENAI_API_KEY"), os.Getenv("PATH")); err != nil {
			os.Exit(25)
		}
	case outputMode:
		if _, err := io.WriteString(os.Stderr, "stderr\n"); err != nil {
			os.Exit(25)
		}
		if _, err := io.WriteString(os.Stdout, strings.Repeat("x", maxOutput*4)); err != nil {
			os.Exit(25)
		}
	case controlMode:
		if _, err := io.WriteString(os.Stdout, "\x1b[31m\r\u202e\x00\n"); err != nil {
			os.Exit(25)
		}
	case exitMode:
		os.Exit(23)
	case "stdin":
		data, err := io.ReadAll(os.Stdin)
		if err != nil || len(data) != 0 {
			os.Exit(27)
		}
	case sleepMode:
		time.Sleep(time.Minute)
	default:
		os.Exit(26)
	}
}
