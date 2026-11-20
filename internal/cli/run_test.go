package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/pkg/terminal"
	"github.com/spf13/cobra"
)

const (
	runSessionFixtureID = "A1b2C3d4E5"
	runHomeCommand      = `printf '%s' "$HOME"`
	runPrintfOK         = "printf ok"
	runEchoOK           = "echo ok"
	runStdoutFixture    = "out"
)

func TestRunCommandExecutesShellTextAndStreams(t *testing.T) {
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
	if err := runCommand(cmd, []string{runHomeCommand}, runner); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if got := stdout.String(); got != home {
		t.Fatalf("stdout = %q, want expanded home %q", got, home)
	}
}

func TestRunCommandDoesNotRequireExecuteSecurityApproval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	runner := terminal.Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	if err := runCommand(cmd, []string{runPrintfOK}, runner); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if stdout.String() != "ok" {
		t.Fatalf("stdout = %q, want ok", stdout.String())
	}
}

func TestParseRunArgsAcceptsOneShellCommand(t *testing.T) {
	t.Parallel()
	opts, err := parseRunArgs([]string{"printf '%s' hello"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != "printf '%s' hello" {
		t.Fatalf("parsed command = %#v, want one shell command", opts.command)
	}
}

func TestParseRunArgsRejectsLegacySeparatorAndMultipleCommandArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--", runEchoOK},
		{runEchoOK, "extra"},
	} {
		if _, err := parseRunArgs(args); err == nil {
			t.Fatalf("parseRunArgs(%q) accepted a non-single-command invocation", args)
		}
	}
}

func TestRunCommandExecutesShellSyntaxInsideZevRun(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	runner := terminal.Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	if err := runCommand(cmd, []string{"printf out; printf err >&2"}, runner); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if stdout.String() != runStdoutFixture {
		t.Fatalf("stdout = %q, want out", stdout.String())
	}
}

func TestBuildInjectedRunCommandPassesShellProgramToZevRun(t *testing.T) {
	t.Parallel()
	got, err := buildInjectedRunInvocation("/opt/zev", `printf '%s' "$HOME"`, "")
	if err != nil {
		t.Fatal(err)
	}
	want := `'/opt/zev' run 'printf '\''%s'\'' "$HOME"'`
	if got != want {
		t.Fatalf("buildInjectedRunCommand() = %q, want %q", got, want)
	}
}

func TestBuildInjectedRunInvocationCarriesShortSessionReference(t *testing.T) {
	t.Parallel()
	session := runSessionFixtureID
	got, err := buildInjectedRunInvocation("/opt/zev", "printf fail", session)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "--session='") || strings.Contains(got, "--conversation=") || strings.Contains(got, "--command-id=") {
		t.Fatalf("injected invocation lost its short session reference: %q", got)
	}
	if !strings.Contains(got, "--session='"+session+"'") {
		t.Fatalf("injected invocation session = %q, want %q", got, session)
	}
	if _, err := buildInjectedRunInvocation("/opt/zev", "printf fail", "too-long-session"); err == nil {
		t.Fatal("invalid session ID was accepted")
	}
}

func TestParseRunArgsAcceptsOnlyShortSessionReference(t *testing.T) {
	t.Parallel()
	session := runSessionFixtureID
	opts, err := parseRunArgs([]string{"--session=" + session, runPrintfOK})
	if err != nil {
		t.Fatal(err)
	}
	if opts.sessionID != session {
		t.Fatalf("parsed session = %q, want %q", opts.sessionID, session)
	}
	for _, value := range []string{"123456789", "12345678901", "!!!!!!!!!!"} {
		if _, err := parseRunArgs([]string{"--session=" + value, runPrintfOK}); err == nil {
			t.Fatalf("invalid session %q was accepted", value)
		}
	}
	if _, err := parseRunArgs([]string{"--conversation=legacy", "--command-id=legacy", runPrintfOK}); err == nil {
		t.Fatal("legacy split session parameters were accepted")
	}
}

func TestParseRunArgsAcceptsProxy(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
		want string
		bad  bool
	}{
		{name: "separate", args: []string{cliProxyFlag, "socks5h://proxy.example:1080", runEchoOK}, want: "socks5h://proxy.example:1080"},
		{name: "equals", args: []string{"--proxy=http://proxy.example:8080", runEchoOK}, want: "http://proxy.example:8080"},
		{name: "missing", args: []string{cliProxyFlag}, bad: true},
		{name: "empty", args: []string{"--proxy=", runEchoOK}, bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseRunArgs(test.args)
			if test.bad {
				if err == nil {
					t.Fatal("invalid proxy argument was accepted")
				}
				return
			}
			if err != nil || got.proxy != test.want {
				t.Fatalf("parseRunArgs() proxy=%q, err=%v; want %q", got.proxy, err, test.want)
			}
		})
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
	for _, option := range []string{"--on-failure=ask|none|analyze|fix", "--log-limit=BYTES", "--session=ID", "--proxy=URL"} {
		if !strings.Contains(output.String(), option) {
			t.Errorf("run help missing %q: %s", option, output.String())
		}
	}
	for _, removed := range []string{"retry", "--max-retries", "--retry-delay", "--exit-code"} {
		if strings.Contains(output.String(), removed) {
			t.Errorf("run help retained removed retry option %q: %s", removed, output.String())
		}
	}
}

func TestParseRunArgsRejectsRetryOptionsAndKeepsSingleAttemptOptions(t *testing.T) {
	t.Parallel()
	opts, err := parseRunArgs([]string{"--on-failure=analyze", "--timeout=1s", runPrintfOK})
	if err != nil {
		t.Fatal(err)
	}
	if opts.onFailure != runFailureAnalyze || opts.command != runPrintfOK || opts.timeout == 0 {
		t.Fatalf("unexpected options: %+v", opts)
	}
	for _, args := range [][]string{
		{"--on-failure=retry", runPrintfOK},
		{"--max-retries=1", runPrintfOK},
		{"--retry-delay=10ms", runPrintfOK},
		{"--exit-code=last", runPrintfOK},
		{"--on-failure=unknown", runPrintfOK},
	} {
		if _, err := parseRunArgs(args); err == nil {
			t.Fatalf("parseRunArgs(%q) accepted removed or invalid invocation", args)
		}
	}
}

func TestRunCommandMakesOnlyOneAttempt(t *testing.T) {
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
	command := []string{"--on-failure=none", fmt.Sprintf(`n=$(cat %q); n=$((n+1)); printf '%%s' "$n" > %q; exit 1`, marker, marker)}
	if err := runCommand(&cobra.Command{}, command, runner); err == nil {
		t.Fatal("runCommand() unexpectedly succeeded")
	}
	data, err := os.ReadFile(filepath.Clean(marker))
	if err != nil || string(data) != "1" {
		t.Fatalf("attempt marker = %q, err=%v; want one attempt", data, err)
	}
}

func TestRunFailurePrintsCapturedChildOutputBeforeReturning(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	runner := terminal.Runner{Stdin: strings.NewReader(""), Stderr: &stderr}
	err := executeRunAttempts(t.Context(), runOptions{
		command:   `printf out; printf err >&2; exit 1`,
		onFailure: runFailureNone, logLimit: 1024,
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
		command:   `printf out; printf err >&2; exit 1`,
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
			if strings.Contains(stderr.String(), "may contain secrets") {
				t.Fatal("recovery emitted removed disclosure warning")
			}
			return nil
		},
	}, runner, &stderr, false)
	if !recovered {
		t.Fatalf("execution error = %v, recovery called = %t", err, recovered)
	}
}

func TestRunDoesNotRecoverOrRetryAfterCancellationOrSignal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		command     string
		cancel      bool
		failureLogs int
	}{
		{name: "canceled", command: "sleep 1", cancel: true, failureLogs: 0},
		{name: "signaled", command: "kill -TERM $$", failureLogs: 1},
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
				command: tc.command, onFailure: runFailureNone, logLimit: 1024,
				recover: func(context.Context, string, error, runOutput) error { recoveryCalls++; return nil },
			}, terminal.Runner{Stderr: &stderr}, &stderr, false)
			if err == nil {
				t.Fatal("interrupted command returned nil error")
			}
			if recoveryCalls != 0 || strings.Count(stderr.String(), "--- command failure log") != tc.failureLogs {
				t.Fatalf("recovery calls = %d, failure logs = %d, want %d", recoveryCalls, strings.Count(stderr.String(), "--- command failure log"), tc.failureLogs)
			}
		})
	}
}

func TestRunFixRecoverySuppressesOriginalFailureAfterSuccessfulDelivery(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	err := executeRunAttempts(t.Context(), runOptions{
		command:   "exit 1",
		onFailure: runFailureFix, logLimit: 1024,
		recover: func(context.Context, string, error, runOutput) error { return nil },
	}, terminal.Runner{Stderr: &stderr}, &stderr, false)
	if err != nil {
		t.Fatalf("successful fix recovery returned original failure: %v", err)
	}
}

func TestRunStartupFailureIsRecoverable(t *testing.T) {
	t.Parallel()
	_, err := runOneAttempt(t.Context(), "command-that-does-not-exist-zev-test", terminal.Runner{}, 1024)
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

func TestRunRecoveryOutputKeepsSectionBlankLines(t *testing.T) {
	t.Parallel()
	var actionSeparator bytes.Buffer
	if err := writeRunRecoveryActionSeparator(&actionSeparator); err != nil {
		t.Fatal(err)
	}
	if actionSeparator.String() != "\n" {
		t.Fatalf("recovery action separator = %q, want one blank line", actionSeparator.String())
	}
	var output bytes.Buffer
	if err := writeRunRecoveryProgress(&output); err != nil {
		t.Fatal(err)
	}
	fetching := locale.Text(locale.KeyQueryFetchingSuggestions)
	want := fetching
	if output.String() != want {
		t.Fatalf("recovery progress = %q, want %q", output.String(), want)
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

func TestRootHidesHelpSubcommandButKeepsHelpFlagsAndCompletion(t *testing.T) {
	t.Parallel()
	root := NewRootCommand("test")
	if cmd, _, err := root.Find([]string{entityHelpFlag[2:]}); err == nil && cmd != nil && cmd.Name() == entityHelpFlag[2:] {
		t.Fatal("help subcommand is publicly registered")
	}
	root.InitDefaultCompletionCmd()
	if cmd, _, err := root.Find([]string{"completion"}); err != nil || cmd == nil || cmd.Name() != "completion" {
		t.Fatalf("completion command missing: cmd=%v err=%v", cmd, err)
	}
	root.InitDefaultHelpFlag()
	if root.Flag("help") == nil {
		t.Fatal("root --help flag missing")
	}
}

func TestManagedDeleteAliases(t *testing.T) {
	t.Parallel()
	for name, command := range map[string]*cobra.Command{managedKindAlias: newAliasRemoveCmd(), managedKindScript: newScriptRemoveCmd()} {
		found := false
		for _, alias := range command.Aliases {
			if alias == "delete" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s remove command does not expose delete alias: %v", name, command.Aliases)
		}
	}
}
