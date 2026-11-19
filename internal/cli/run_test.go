package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/pkg/terminal"
	"github.com/spf13/cobra"
)

const (
	runTestPrintf    = "printf"
	runTestRetryFlag = "--on-failure=retry"
)

func TestRunArgvPreservesLiteralArgumentsAndStreams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	zevDir := filepath.Join(home, ".zev")
	if err := os.Mkdir(zevDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := []byte("version: 1\nactive_provider: openai\nproviders:\n  openai:\n    type: openai\nsecurity:\n  mode: bypass\n")
	if err := os.WriteFile(filepath.Join(zevDir, "config.yaml"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetErr(&stderr)
	runner := terminal.Runner{Stdout: &stdout, Stderr: &stderr}
	if err := runArgv(cmd, []string{"--", runTestPrintf, "%s", "$HOME"}, runner); err != nil {
		t.Fatalf("runArgv() error = %v", err)
	}
	if got := stdout.String(); got != "$HOME" {
		t.Fatalf("stdout = %q, want literal argument", got)
	}
}

func TestRunArgvDoesNotRequireExecuteSecurityApproval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	runner := terminal.Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	if err := runArgv(cmd, []string{"--", "printf", "ok"}, runner); err != nil {
		t.Fatalf("runArgv() error = %v", err)
	}
	if stdout.String() != "ok" {
		t.Fatalf("stdout = %q, want ok", stdout.String())
	}
}

func TestBuildInjectedRunCommandPreservesShellProgramAsOneArgument(t *testing.T) {
	t.Parallel()
	got, err := buildInjectedRunInvocation("/opt/zev", "/bin/sh", `printf '%s' "$HOME"`, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := `'/opt/zev' run -- '/bin/sh' -c 'printf '\''%s'\'' "$HOME"'`
	if got != want {
		t.Fatalf("buildInjectedRunCommand() = %q, want %q", got, want)
	}
}

func TestBuildInjectedRunInvocationCarriesExactHistoryReferences(t *testing.T) {
	t.Parallel()
	conversationID := "a5aeb2b8-42c3-4f14-9a7b-fba914ef32c1"
	commandID := "b6d3fd36-02de-4d63-a057-1845876218e2"
	got, err := buildInjectedRunInvocation("/opt/zev", "/bin/sh", "printf fail", conversationID, commandID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "--conversation='"+conversationID+"'") || !strings.Contains(got, "--command-id='"+commandID+"'") || !strings.Contains(got, " -- '/bin/sh' -c ") {
		t.Fatalf("injected invocation lost its history references or shell argv: %q", got)
	}
	if _, err := buildInjectedRunInvocation("/opt/zev", "/bin/sh", "printf fail", "not-a-uuid", ""); err == nil {
		t.Fatal("non-canonical conversation reference was accepted")
	}
}

func TestRootRegistersRunCommand(t *testing.T) {
	t.Parallel()
	root := NewRootCommand("test")
	if _, _, err := root.Find([]string{"run"}); err != nil {
		t.Fatalf("run command not registered: %v", err)
	}
}

func TestRunHelpDocumentsRecoveryActionsAndLogLimit(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	if err := writeRunHelp(cmd); err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"--on-failure=ask|none|retry|analyze|fix", "--log-limit=BYTES"} {
		if !strings.Contains(output.String(), option) {
			t.Errorf("run help missing %q: %s", option, output.String())
		}
	}
}

func TestParseRunArgsValidatesSeparatorAndRetryPolicy(t *testing.T) {
	t.Parallel()
	opts, err := parseRunArgs([]string{runTestRetryFlag, "--max-retries=2", "--retry-delay=10ms", "--timeout=1s", "--", runTestPrintf, "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.onFailure != runFailureRetry || opts.maxRetries != 2 || len(opts.command) != 2 || opts.timeout == 0 {
		t.Fatalf("unexpected options: %+v", opts)
	}
	analyze, err := parseRunArgs([]string{"--on-failure=analyze", "--", runTestPrintf, "ok"})
	if err != nil || analyze.onFailure != runFailureAnalyze {
		t.Fatalf("analyze recovery option = %+v, %v", analyze, err)
	}
	for _, args := range [][]string{
		{runTestPrintf, "ok"},
		{runTestRetryFlag, "--max-retries=0", "--", runTestPrintf, "ok"},
		{"--on-failure=unknown", "--", runTestPrintf, "ok"},
	} {
		if _, err := parseRunArgs(args); err == nil {
			t.Fatalf("parseRunArgs(%q) accepted invalid invocation", args)
		}
	}
}

func TestRunArgvRetriesOriginalLiteralArgv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	zevDir := filepath.Join(home, ".zev")
	if err := os.Mkdir(zevDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := []byte("version: 1\nactive_provider: openai\nproviders:\n  openai:\n    type: openai\nsecurity:\n  mode: bypass\n")
	if err := os.WriteFile(filepath.Join(zevDir, "config.yaml"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "attempts")
	if err := os.WriteFile(marker, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: &stderr}
	command := []string{"--on-failure=retry", "--max-retries=1", "--", "sh", "-c", "n=$(cat \"$1\"); n=$((n+1)); printf '%s' \"$n\" > \"$1\"; test \"$n\" -ge 2", "sh", marker}
	if err := runArgv(&cobra.Command{}, command, runner); err != nil {
		t.Fatalf("runArgv() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(marker))
	if err != nil || string(data) != "2" {
		t.Fatalf("attempt marker = %q, err=%v", data, err)
	}
}

func TestRunFailurePrintsCapturedChildOutputBeforeReturning(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	runner := terminal.Runner{Stdin: strings.NewReader(""), Stderr: &stderr}
	err := executeRunAttempts(t.Context(), runOptions{
		command:   []string{"sh", "-c", "printf out; printf err >&2; exit 1"},
		onFailure: runFailureNone, maxRetries: 0, logLimit: 1024,
	}, runner, &stderr, false)
	if err == nil {
		t.Fatal("failed command returned nil error")
	}
	if !strings.Contains(stderr.String(), "out") || !strings.Contains(stderr.String(), "err") {
		t.Fatalf("failure output was not replayed: %q", stderr.String())
	}
}

func TestRunFailureRecoveryReceivesCapturedContextAfterLog(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	recovered := false
	runner := terminal.Runner{Stdout: &stdout, Stderr: &stderr}
	err := executeRunAttempts(t.Context(), runOptions{
		command:   []string{"sh", "-c", "printf out; printf err >&2; exit 1"},
		onFailure: runFailureAnalyze, logLimit: 1024,
		recover: func(_ context.Context, action string, commandErr error, output runOutput) error {
			recovered = true
			if action != runFailureAnalyze || commandErr == nil || !strings.Contains(commandErr.Error(), "exit status 1") {
				t.Fatalf("recovery action/error = %q, %v", action, commandErr)
			}
			if output.stdout != "out" || output.stderr != "err" || output.stdoutTruncated || output.stderrTruncated {
				t.Fatalf("captured recovery output = %+v", output)
			}
			if !strings.Contains(stderr.String(), "--- end command failure log ---") {
				t.Fatal("recovery started before the failure log was displayed")
			}
			if !strings.Contains(stderr.String(), "may contain secrets") {
				t.Fatal("recovery started without warning that captured output may disclose secrets")
			}
			return nil
		},
	}, runner, &stderr, false)
	if err == nil || !recovered {
		t.Fatalf("execution error = %v, recovery called = %t", err, recovered)
	}
}

func TestRunDoesNotRecoverOrRetryAfterCancellationOrSignal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		command []string
		cancel  bool
	}{
		{name: "canceled", command: []string{"sh", "-c", "sleep 1"}, cancel: true},
		{name: "signaled", command: []string{"sh", "-c", "kill -TERM $$"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			cancel := func() {}
			if tc.cancel {
				var cancelCtx context.CancelFunc
				ctx, cancelCtx = context.WithTimeout(ctx, 50*time.Millisecond)
				cancel = cancelCtx
			}
			defer cancel()
			var stderr bytes.Buffer
			recoveryCalls := 0
			err := executeRunAttempts(ctx, runOptions{
				command: tc.command, onFailure: runFailureRetry, maxRetries: 2, logLimit: 1024,
				recover: func(context.Context, string, error, runOutput) error { recoveryCalls++; return nil },
			}, terminal.Runner{Stderr: &stderr}, &stderr, false)
			if err == nil {
				t.Fatal("interrupted command returned nil error")
			}
			if recoveryCalls != 0 || strings.Count(stderr.String(), "--- command failure log") != 1 {
				t.Fatalf("recovery calls = %d, failure logs = %d", recoveryCalls, strings.Count(stderr.String(), "--- command failure log"))
			}
		})
	}
}

func TestRunStartupFailureIsRecoverable(t *testing.T) {
	t.Parallel()
	missingCommand := filepath.Join(t.TempDir(), "missing-command")
	_, err := runOneAttempt(t.Context(), []string{missingCommand}, terminal.Runner{}, 1024)
	if err == nil {
		t.Fatal("missing executable returned nil error")
	}
	if !recoverableRunFailure(t.Context(), err) {
		t.Fatalf("startup failure %v was not considered recoverable", err)
	}
}

func TestRunRecoverySuggestionsEscapeTerminalControls(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	response := &domain.OptionsResponse{
		IsValid:  true,
		Commands: []domain.Command{{Command: "printf \x1b[31m", ShortExplanation: "explain \x1b[0m"}},
	}
	if err := writeRunRecoverySuggestions(&output, response, runFailureFix); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(output.String(), '\x1b') || !strings.Contains(output.String(), `\x1b`) || !strings.Contains(output.String(), "not executed") {
		t.Fatalf("recovery output was not inert and terminal-safe: %q", output.String())
	}
}

func TestBoundedCaptureRetainsOnlyTail(t *testing.T) {
	t.Parallel()
	capture := newBoundedCapture(4)
	if _, err := capture.Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	got, truncated := capture.snapshot()
	if got != "cdef" || !truncated {
		t.Fatalf("snapshot = %q, truncated=%t; want tail %q", got, truncated, "cdef")
	}
}
