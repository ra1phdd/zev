package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/security"
	"github.com/ra1phdd/zev/pkg/terminal"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	runFailureAsk           = "ask"
	runFailureNone          = "none"
	runFailureAnalyze       = "analyze"
	runFailureFix           = "fix"
	runToolApprovalFlagName = "--tool-approval"
)

type runOptions struct {
	command    string
	sessionID  string
	proxy      string
	logLimit   int
	recover    func(context.Context, string, error, runOutput) error
	onFailure  string
	noInteract bool
	timeout    time.Duration
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run [options] <command>",
		Short:              locale.Text(locale.KeyCliRunShort),
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if isRunHelp(args) {
				return writeRunHelp(cmd)
			}
			return runCommand(cmd, args, terminal.DefaultRunner())
		},
	}
}

func runCommand(cmd *cobra.Command, args []string, runner terminal.Runner) error {
	if cmd.Context() == nil {
		cmd.SetContext(context.Background())
	}
	opts, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	restoreSessionLanguage := setRunSessionLanguage(cmd.Context(), opts.sessionID)
	defer restoreSessionLanguage()
	opts.recover = func(ctx context.Context, action string, commandErr error, output runOutput) error {
		return runRecovery(ctx, cmd, opts, action, commandErr, output)
	}
	ctx := contextWithCommandProxy(cmd.Context(), opts.proxy)
	return executeRunAttempts(ctx, opts, runner, cmd.ErrOrStderr(), !opts.noInteract && interactiveRunInput(cmd))
}

func injectedRunInvocation(command, sessionID string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("zev run wrapper injection is not supported on Windows; disable command_wrapper_enabled")
	}
	zevPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve zev executable for run wrapper: %w", err)
	}
	zevPath, err = filepath.Abs(zevPath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute zev executable path: %w", err)
	}
	return buildInjectedRunInvocation(zevPath, command, sessionID)
}

func buildInjectedRunInvocation(zevPath, command, sessionID string) (string, error) {
	if !filepath.IsAbs(zevPath) {
		return "", errors.New("run wrapper requires an absolute executable path")
	}
	if strings.ContainsRune(zevPath, 0) || strings.ContainsRune(command, 0) {
		return "", errors.New("run wrapper arguments must not contain NUL")
	}
	if sessionID != "" {
		if err := validateRunSessionID(sessionID); err != nil {
			return "", err
		}
	}
	invocation := shellQuote(zevPath) + " run"
	if sessionID != "" {
		invocation += " --session=" + shellQuote(sessionID)
	}
	return invocation + " " + shellQuote(command), nil
}

//nolint:gocognit // The standalone parser keeps all option validation in one fail-closed boundary.
func parseRunArgs(args []string) (runOptions, error) {
	opts := runOptions{onFailure: runFailureAsk, logLimit: 1 << 20}
	commandIndex := -1
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return runOptions{}, errors.New("run no longer accepts --; quote the shell command as one argument")
		}
		if !strings.HasPrefix(arg, "--") {
			commandIndex = i
			break
		}
		if arg == "--no-interactive" {
			opts.noInteract = true
			continue
		}

		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case cliProxyFlag:
			if !hasValue {
				if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
					return runOptions{}, errors.New("--proxy requires a URL")
				}
				i++
				value = args[i]
			}
			if value == "" {
				return runOptions{}, errors.New("--proxy requires a URL")
			}
			opts.proxy = value
		case "--on-failure":
			if !hasValue {
				return runOptions{}, errors.New("--on-failure requires a value")
			}
			switch value {
			case runFailureAsk, runFailureNone, runFailureAnalyze, runFailureFix:
				opts.onFailure = value
			default:
				return runOptions{}, fmt.Errorf("invalid --on-failure value %q", value)
			}
		case "--timeout":
			if !hasValue || !parseDuration(value, 24*time.Hour, &opts.timeout) {
				return runOptions{}, errors.New("--timeout must be a duration from 0s to 24h")
			}
		case "--log-limit":
			if !hasValue || !parseNonNegativeInt(value, &opts.logLimit) || opts.logLimit < 1024 || opts.logLimit > 16<<20 {
				return runOptions{}, errors.New("--log-limit must be between 1024 and 16777216 bytes")
			}
		case "--session":
			if !hasValue {
				return runOptions{}, errors.New("--session requires a compact session ID")
			}
			if err := validateRunSessionID(value); err != nil {
				return runOptions{}, fmt.Errorf("invalid --session: %w", err)
			}
			opts.sessionID = value
		case "--events-file", "--tools", "--no-tools", runToolApprovalFlagName, "--no-history", "--no-context":
			return runOptions{}, fmt.Errorf("run option %s is not implemented", name)
		default:
			return runOptions{}, fmt.Errorf("unknown run option %q", name)
		}
	}
	if commandIndex < 0 {
		return runOptions{}, errors.New("run requires one shell command")
	}
	if commandIndex != len(args)-1 {
		return runOptions{}, errors.New("run accepts exactly one shell command")
	}
	if args[commandIndex] == "" {
		return runOptions{}, errors.New("run requires a non-empty shell command")
	}
	opts.command = args[commandIndex]
	return opts, nil
}

func parseNonNegativeInt(value string, target *int) bool {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return false
	}
	*target = parsed
	return true
}

func parseDuration(value string, limit time.Duration, target *time.Duration) bool {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 || parsed > limit {
		return false
	}
	*target = parsed
	return true
}

func homeDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func runAssessmentCertainty(known bool) security.Certainty {
	if known {
		return security.CertaintyComplete
	}
	return security.CertaintyUnknown
}

func executeRunAttempts(ctx context.Context, opts runOptions, runner terminal.Runner, failureOut io.Writer, interactive bool) error {
	attemptCtx := ctx
	cancel := func() {}
	if opts.timeout > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, opts.timeout)
	}
	output, commandErr := runOneAttempt(attemptCtx, opts.command, runner, opts.logLimit)
	cancel()
	if commandErr == nil {
		return nil
	}
	if ctx.Err() != nil || isInterruptedProcess(commandErr) {
		return context.Canceled
	}
	if !recoverableRunFailure(ctx, commandErr) {
		if !interactive {
			if err := writeRunFailureLog(failureOut, output); err != nil {
				return errors.Join(commandErr, err)
			}
		}
		if err := writeRunStatus(failureOut, locale.Text(locale.KeyCliRunRecoverySkipped)); err != nil {
			return errors.Join(commandErr, err)
		}
		return commandErr
	}
	action := opts.onFailure
	if opts.onFailure == runFailureAsk && interactive {
		var err error
		action, err = selectRunFailureAction(ctx, failureOut)
		if err != nil {
			return errors.Join(commandErr, err)
		}
	}
	if !interactive {
		if err := writeRunFailureLog(failureOut, output); err != nil {
			return errors.Join(commandErr, err)
		}
	}
	if err := runFailureRecovery(ctx, opts, action, commandErr, output, failureOut); err != nil {
		return errors.Join(commandErr, err)
	}
	return runFailureOutcome(action, commandErr)
}

func runFailureOutcome(action string, commandErr error) error {
	if action == runFailureFix {
		return nil
	}
	return commandErr
}

func runFailureRecovery(ctx context.Context, opts runOptions, action string, commandErr error, output runOutput, failureOut io.Writer) error {
	if action == runFailureNone {
		return nil
	}
	if action != runFailureAnalyze && action != runFailureFix {
		return nil
	}
	if opts.recover == nil {
		return writeRunStatus(failureOut, locale.Text(locale.KeyCliRunRecoveryUnavailable))
	}
	if err := opts.recover(ctx, action, commandErr, output); err != nil {
		if writeErr := writeRunStatus(failureOut, locale.Text(locale.KeyCliRunRecoveryFailed)); writeErr != nil {
			return errors.Join(err, writeErr)
		}
	}
	return nil
}

func selectRunFailureAction(ctx context.Context, output io.Writer) (string, error) {
	if err := writeRunRecoveryActionSeparator(output); err != nil {
		return "", err
	}
	items := []list.Item{
		list.NewSimpleItem(locale.Text(locale.KeyCliRunRecoveryFix), locale.Text(locale.KeyCliRunRecoveryFixDescription)),
		list.NewSimpleItem(locale.Text(locale.KeyCliRunRecoveryAnalyze), locale.Text(locale.KeyCliRunRecoveryAnalyzeDescription)),
		list.NewSimpleItem(locale.Text(locale.KeyCliRunRecoveryDoNothing), locale.Text(locale.KeyCliRunRecoveryDoNothingDescription)),
	}
	selected, err := list.SelectContext(ctx, list.SelectOptions{
		Title:  runRecoveryActionTitle(),
		Items:  items,
		Output: output,
	})
	if err != nil {
		return "", fmt.Errorf("select run recovery action: %w", err)
	}
	item, ok := selected.(list.SimpleItem)
	if !ok {
		return "", fmt.Errorf("unexpected recovery selection %T", selected)
	}
	switch item.Title() {
	case locale.Text(locale.KeyCliRunRecoveryFix):
		return runFailureFix, nil
	case locale.Text(locale.KeyCliRunRecoveryAnalyze):
		return runFailureAnalyze, nil
	case locale.Text(locale.KeyCliRunRecoveryDoNothing):
		return runFailureNone, nil
	default:
		return "", errors.New("unknown recovery selection")
	}
}

func writeRunRecoveryActionSeparator(out io.Writer) error {
	return writeBlankLine(out)
}

func runRecoveryActionTitle() string {
	return locale.Text(locale.KeyCliRunRecoveryTitle)
}

func writeRunRecoveryProgress(out io.Writer) error {
	if err := writeQueryOutput(out, "%s", locale.Text(locale.KeyQueryFetchingSuggestions)); err != nil {
		return err
	}
	return nil
}

func writeBlankLine(out io.Writer) error {
	if _, err := fmt.Fprintln(out); err != nil {
		return fmt.Errorf("write section separator: %w", err)
	}
	return nil
}

func writeRunStatus(out io.Writer, status string) error {
	if _, err := fmt.Fprintln(out, status); err != nil {
		return fmt.Errorf("write run status: %w", err)
	}
	return nil
}

func interactiveRunInput(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func runOneAttempt(ctx context.Context, command string, runner terminal.Runner, logLimit int) (runOutput, error) {
	stdoutCapture, stderrCapture := newRunOutputCapture(logLimit)
	runner.Stdout = captureWriter(runner.Stdout, stdoutCapture)
	runner.Stderr = captureWriter(runner.Stderr, stderrCapture)
	if err := runner.RunShell(ctx, terminal.DefaultShell(), command); err != nil {
		return snapshotRunOutput(stdoutCapture, stderrCapture), fmt.Errorf("run command: %w", err)
	}
	return snapshotRunOutput(stdoutCapture, stderrCapture), nil
}

func recoverableRunFailure(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return recoverableRunError(err)
}

func recoverableRunError(err error) bool {
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !recoverableRunError(child) {
				return false
			}
		}
		return true
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode() >= 0
	}
	if pathErr, ok := errors.AsType[*os.PathError](err); ok {
		return pathErr != nil
	}
	if execErr, ok := errors.AsType[*exec.Error](err); ok {
		if errors.Is(execErr.Err, exec.ErrNotFound) {
			return true
		}
		return recoverableRunError(execErr.Err)
	}
	var wrapped interface{ Unwrap() error }
	if errors.As(err, &wrapped) {
		return recoverableRunError(wrapped.Unwrap())
	}
	return false
}

func isRunHelp(args []string) bool {
	return len(args) == 1 && (args[0] == entityHelpFlag || args[0] == "-h")
}

func writeRunHelp(cmd *cobra.Command) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), `Usage: zev run [options] command

Options:
  --on-failure=ask|none|analyze|fix
                               Failure policy; analyze/fix send captured output to the configured provider
  --timeout=D                  Per-attempt timeout (0-24h)
  --log-limit=BYTES            Maximum captured stdout/stderr combined per attempt (1 KiB-16 MiB)
  --session=ID                  10-character key for the linked conversation/command
  --proxy=URL                   Route recovery requests through an HTTP(S) or SOCKS5 proxy
  --no-interactive             Disable recovery prompts

The command is executed by the configured shell. Quote the entire command as one argument. Recovery sends captured output, which may contain secrets, to the configured provider. Suggestions are never executed automatically.`)
	if err != nil {
		return fmt.Errorf("write run help: %w", err)
	}
	return nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
