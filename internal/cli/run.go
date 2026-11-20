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
	command    []string
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
		Use:                "run -- command [args...]",
		Short:              locale.Text(locale.KeyCliRunShort),
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if isRunHelp(args) {
				return writeRunHelp(cmd)
			}
			return runArgv(cmd, args, terminal.DefaultRunner())
		},
	}
}

func runArgv(cmd *cobra.Command, args []string, runner terminal.Runner) error {
	if cmd.Context() == nil {
		cmd.SetContext(context.Background())
	}
	opts, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	opts.recover = func(ctx context.Context, action string, commandErr error, output runOutput) error {
		return runRecovery(ctx, cmd, opts, action, commandErr, output)
	}
	ctx := contextWithCommandProxy(cmd.Context(), opts.proxy)
	return executeRunAttemptsWithInput(ctx, opts, runner, cmd.ErrOrStderr(), cmd.InOrStdin(), !opts.noInteract && interactiveRunInput(cmd))
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
	shell, err := exec.LookPath(terminal.DefaultShell())
	if err != nil {
		return "", fmt.Errorf("resolve shell for run wrapper: %w", err)
	}
	shell, err = filepath.Abs(shell)
	if err != nil {
		return "", fmt.Errorf("resolve absolute shell path: %w", err)
	}
	switch filepath.Base(shell) {
	case "sh", "bash", "dash", "zsh":
	default:
		return "", fmt.Errorf("shell %q is not supported for POSIX run wrapper quoting", filepath.Base(shell))
	}
	return buildInjectedRunInvocation(zevPath, shell, command, sessionID)
}

func buildInjectedRunInvocation(zevPath, shell, command, sessionID string) (string, error) {
	if !filepath.IsAbs(zevPath) || !filepath.IsAbs(shell) {
		return "", errors.New("run wrapper requires absolute executable and shell paths")
	}
	if strings.ContainsRune(zevPath, 0) || strings.ContainsRune(shell, 0) || strings.ContainsRune(command, 0) {
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
	return invocation + " -- " + shellQuote(shell) + " -c " + shellQuote(command), nil
}

//nolint:gocognit // The standalone parser keeps all pre-separator validation in one fail-closed boundary.
func parseRunArgs(args []string) (runOptions, error) {
	opts := runOptions{onFailure: runFailureAsk, logLimit: 1 << 20}
	separator := -1
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			separator = i
			break
		}
		if arg == "--no-interactive" {
			opts.noInteract = true
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			return runOptions{}, fmt.Errorf("run option must precede --: %q", arg)
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
	if separator < 0 {
		return runOptions{}, errors.New("run requires -- before the executable")
	}
	if separator == len(args)-1 {
		return runOptions{}, errors.New("run requires an executable after --")
	}
	opts.command = append([]string(nil), args[separator+1:]...)
	if opts.command[0] == "" {
		return runOptions{}, errors.New("run requires a non-empty executable")
	}
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
	return executeRunAttemptsWithInput(ctx, opts, runner, failureOut, os.Stdin, interactive)
}

func executeRunAttemptsWithInput(ctx context.Context, opts runOptions, runner terminal.Runner, failureOut io.Writer, input io.Reader, interactive bool) error {
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
	if err := writeRunFailureLog(failureOut, output); err != nil {
		return errors.Join(commandErr, err)
	}
	if !recoverableRunFailure(ctx, commandErr) {
		if err := writeRunStatus(failureOut, "Recovery skipped because the process was interrupted or its outcome could not be confirmed."); err != nil {
			return errors.Join(commandErr, err)
		}
		return commandErr
	}
	action := opts.onFailure
	if opts.onFailure == runFailureAsk && interactive {
		var err error
		action, err = selectRunRecovery(ctx, input)
		if err != nil {
			return errors.Join(commandErr, err)
		}
	}
	if err := runFailureRecovery(ctx, opts, action, commandErr, output, failureOut); err != nil {
		return errors.Join(commandErr, err)
	}
	return commandErr
}

func runFailureRecovery(ctx context.Context, opts runOptions, action string, commandErr error, output runOutput, failureOut io.Writer) error {
	if action != runFailureAnalyze && action != runFailureFix {
		return nil
	}
	if opts.recover == nil {
		return writeRunStatus(failureOut, "AI recovery is unavailable for this invocation.")
	}
	if err := writeRunStatus(failureOut, "Warning: captured command output will be sent to the configured provider and may contain secrets."); err != nil {
		return err
	}
	if err := opts.recover(ctx, action, commandErr, output); err != nil {
		if writeErr := writeRunStatus(failureOut, "AI recovery failed; the command result is unchanged."); writeErr != nil {
			return errors.Join(err, writeErr)
		}
	}
	return nil
}

func writeRunStatus(out io.Writer, status string) error {
	if _, err := fmt.Fprintln(out, status); err != nil {
		return fmt.Errorf("write run status: %w", err)
	}
	return nil
}

type runRecoveryItem struct {
	action string
	title  string
	detail string
}

func (i runRecoveryItem) Title() string       { return i.title }
func (i runRecoveryItem) Description() string { return i.detail }
func (i runRecoveryItem) IsWarning() bool     { return false }

func selectRunRecovery(ctx context.Context, input io.Reader) (string, error) {
	selected, err := list.SelectContext(ctx, list.SelectOptions{
		Title: locale.Text(locale.KeyCliRunRecoveryTitle),
		Input: input,
		Items: []list.Item{
			runRecoveryItem{action: runFailureNone, title: locale.Text(locale.KeyCliRunRecoveryDoNothing), detail: locale.Text(locale.KeyCliRunRecoveryDoNothingDescription)},
			runRecoveryItem{action: runFailureAnalyze, title: locale.Text(locale.KeyCliRunRecoveryAnalyze), detail: locale.Text(locale.KeyCliRunRecoveryAnalyzeDescription)},
			runRecoveryItem{action: runFailureFix, title: locale.Text(locale.KeyCliRunRecoveryFix), detail: locale.Text(locale.KeyCliRunRecoveryFixDescription)},
		},
	})
	if err != nil {
		return "", fmt.Errorf("select run recovery action: %w", err)
	}
	item, ok := selected.(runRecoveryItem)
	if !ok {
		return "", errors.New("invalid run recovery selection")
	}
	return item.action, nil
}

func interactiveRunInput(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func runOneAttempt(ctx context.Context, args []string, runner terminal.Runner, logLimit int) (runOutput, error) {
	stdoutCapture, stderrCapture := newRunOutputCapture(logLimit)
	runner.Stdout = captureWriter(runner.Stdout, stdoutCapture)
	runner.Stderr = captureWriter(runner.Stderr, stderrCapture)
	if err := runner.RunArgv(ctx, args[0], args[1:]); err != nil {
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
	_, err := fmt.Fprintln(cmd.OutOrStdout(), `Usage: zev run [options] -- command [args...]

Options:
  --on-failure=ask|none|analyze|fix
                               Failure policy; analyze/fix send captured output to the configured provider
  --timeout=D                  Per-attempt timeout (0-24h)
  --log-limit=BYTES            Maximum captured stdout/stderr combined per attempt (1 KiB-16 MiB)
  --session=ID                  10-character key for the linked conversation/command
  --proxy=URL                   Route recovery requests through an HTTP(S) or SOCKS5 proxy
  --no-interactive             Disable recovery prompts

Recovery sends captured output, which may contain secrets, to the configured provider. Suggestions are never executed automatically. Arguments after -- are passed literally; no shell parsing is performed.`)
	if err != nil {
		return fmt.Errorf("write run help: %w", err)
	}
	return nil
}

func makeAssessmentArgv(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
