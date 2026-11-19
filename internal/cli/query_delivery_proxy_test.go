package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/security"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"

	"github.com/ra1phdd/zev/pkg/terminal"
)

func TestExecuteDeliveryWarnsBeforeStartingTheChild(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	outputErr := errors.New("warning unavailable")
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome

	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "printf should-not-run", IsDangerous: true, DangerousExplanation: "warning"},
		mode:    executionModeExecute, out: &stdout, errOut: failingOutput{err: outputErr},
		logger: setupLogger(0), runner: &runner, outcome: &outcome,
	})
	if !errors.Is(err, outputErr) {
		t.Fatalf("error %v", err)
	}
	if stdout.Len() != 0 || outcome.ExecutedAt != nil || outcome.ExecutionExitCode != nil || outcome.DeliveryOutcome == queryDeliveryExecuted {
		t.Fatalf("child ran after warning failure: output=%q outcome=%+v", stdout.String(), outcome)
	}
}

func TestExecuteDeliveryHonorsConfiguredPolicyDeny(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome
	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "printf should-not-run"}, mode: executionModeExecute,
		securityMode: security.ModeNormal, securityPolicy: security.Policy{Mode: security.ModeNormal, Rules: []security.Rule{{Decision: security.DecisionDeny, Sources: []security.Source{security.SourceQuery}}}},
		out: &stdout, errOut: io.Discard, runner: &runner, outcome: &outcome,
	})
	if err == nil || stdout.Len() != 0 || outcome.ExecutedAt != nil {
		t.Fatalf("configured deny was bypassed: err=%v output=%q outcome=%+v", err, stdout.String(), outcome)
	}
}

func TestExecuteDeliveryForwardsChildStreams(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: &stderr}
	var outcome historyservice.SelectionOutcome

	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "printf out; printf err >&2"},
		mode:    executionModeExecute, out: &stdout, errOut: &stderr,
		securityMode: security.ModeBypass,
		logger:       setupLogger(0), runner: &runner, outcome: &outcome,
	})
	if err != nil {
		t.Fatalf("error %v", err)
	}
	if stdout.String() != "out" || stderr.String() != "err" {
		t.Fatalf("streams = stdout %q stderr %q", stdout.String(), stderr.String())
	}
	if outcome.DeliveryOutcome != queryDeliveryExecuted || outcome.ExecutionExitCode == nil || *outcome.ExecutionExitCode != 0 {
		t.Fatalf("outcome %+v", outcome)
	}
}

func TestExecuteDeliveryRequiresConfirmationInNormalMode(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome
	confirmed := false

	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "project-reader should-not-run", ShortExplanation: "Read project files"},
		mode:    executionModeExecute, out: &stdout, errOut: io.Discard,
		securityMode: security.ModeNormal, runner: &runner,
		userRequest: "list project files",
		confirm:     func(context.Context, *domain.Command) (bool, error) { confirmed = true; return false, nil }, outcome: &outcome,
	})
	if err == nil || confirmed || stdout.Len() != 0 || outcome.ExecutedAt != nil {
		t.Fatalf("unknown command without a review provider was not refused: err=%v confirmed=%t output=%q outcome=%+v", err, confirmed, stdout.String(), outcome)
	}
}

func TestExecuteDeliveryRequiresConfirmationForDangerousAction(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome

	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "rm -rf should-not-run", IsDangerous: true, DangerousExplanation: "danger"},
		mode:    executionModeExecute, out: &stdout, errOut: io.Discard,
		securityMode: security.ModeNormal, runner: &runner, outcome: &outcome,
	})
	if err == nil || stdout.Len() != 0 || outcome.ExecutedAt != nil {
		t.Fatalf("dangerous command was not gated: err=%v output=%q outcome=%+v", err, stdout.String(), outcome)
	}
}

func TestExecuteDeliveryUsesLocalDangerAssessment(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome
	confirmed := false

	err := deliverQueryCommand(t.Context(), queryDelivery{
		command:      &domain.Command{Command: "rm -rf should-not-run"},
		mode:         executionModeExecute,
		securityMode: security.ModeNormal,
		out:          &stdout,
		errOut:       io.Discard,
		runner:       &runner,
		confirm:      func(context.Context, *domain.Command) (bool, error) { confirmed = true; return false, nil },
		outcome:      &outcome,
	})
	if err == nil || !confirmed || stdout.Len() != 0 {
		t.Fatalf("local dangerous assessment was bypassed: err=%v confirmed=%t output=%q", err, confirmed, stdout.String())
	}
}

func TestExecuteDeliveryBypassSkipsConfirmation(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome
	confirmed := false

	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "printf bypassed", IsDangerous: true, DangerousExplanation: "danger"},
		mode:    executionModeExecute, out: &stdout, errOut: io.Discard,
		securityMode: security.ModeBypass, runner: &runner,
		confirm: func(context.Context, *domain.Command) (bool, error) { confirmed = true; return false, nil }, outcome: &outcome,
	})
	if err != nil || confirmed || stdout.String() != "bypassed" || outcome.DeliveryOutcome != queryDeliveryExecuted {
		t.Fatalf("bypass did not run proxy: err=%v confirmed=%t output=%q outcome=%+v", err, confirmed, stdout.String(), outcome)
	}
}

func TestExecuteDeliveryTreatsProviderDangerAsAdvisory(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome
	confirmed := false

	err := deliverQueryCommand(t.Context(), queryDelivery{
		command:      &domain.Command{Command: "printf provider-warning", IsDangerous: true, DangerousExplanation: "provider warning"},
		mode:         executionModeExecute,
		securityMode: security.ModeNormal,
		out:          &stdout,
		errOut:       io.Discard,
		runner:       &runner,
		confirm:      func(context.Context, *domain.Command) (bool, error) { confirmed = true; return false, nil },
		outcome:      &outcome,
	})
	if err != nil || confirmed || stdout.String() != "provider-warning" || outcome.DeliveryOutcome != queryDeliveryExecuted {
		t.Fatalf("provider advisory changed execution decision: err=%v confirmed=%t output=%q outcome=%+v", err, confirmed, stdout.String(), outcome)
	}
}

func TestInjectDeliveryDoesNotPrintCommandOnSuccess(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	var outcome historyservice.SelectionOutcome
	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "printf 'literal'"}, mode: executionModeInject,
		out: &stdout, errOut: &stderr,
		inject: func(string) error { return nil }, outcome: &outcome,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("successful injection printed output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestInjectDeliveryPrintsCommandOnlyAfterInjectionFailure(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var outcome historyservice.SelectionOutcome
	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: "printf 'literal'"}, mode: executionModeInject,
		out: &stdout, errOut: io.Discard, logger: setupLogger(0),
		inject: func(string) error { return errors.New("not a terminal") }, outcome: &outcome,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "printf 'literal'") {
		t.Fatalf("fallback=%q, want exact command disclosure", stdout.String())
	}
	if outcome.DeliveryOutcome != queryDeliveryPrinted {
		t.Fatalf("delivery outcome=%q, want %q", outcome.DeliveryOutcome, queryDeliveryPrinted)
	}
}

func TestInjectDeliveryUsesRunWrapperWhenEnabled(t *testing.T) {
	t.Parallel()
	const command = `printf '%s' "$HOME"`
	var injected string
	var outcome historyservice.SelectionOutcome
	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: command}, mode: executionModeInject,
		wrapInject: true, sessionID: runSessionFixtureID,
		logger: setupLogger(0), out: io.Discard, errOut: io.Discard,
		inject: func(value string) error { injected = value; return nil }, outcome: &outcome,
	})
	if err != nil {
		t.Fatalf("deliverQueryCommand() error = %v", err)
	}
	if !strings.Contains(injected, " run --session='") || strings.Contains(injected, "--conversation=") || strings.Contains(injected, "--command-id=") || !strings.HasSuffix(injected, " -c "+shellQuote(command)) {
		t.Fatalf("injected command %q does not wrap the original shell program", injected)
	}
	if outcome.DeliveryOutcome != "injected" {
		t.Fatalf("delivery outcome = %q, want injected", outcome.DeliveryOutcome)
	}
}

func TestInjectDeliveryDoesNotUseRunWrapperWhenDisabled(t *testing.T) {
	t.Parallel()
	const command = `printf '%s' "$HOME"`
	var injected string
	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: command}, mode: executionModeInject,
		logger: setupLogger(0), out: io.Discard, errOut: io.Discard,
		inject: func(value string) error { injected = value; return nil },
	})
	if err != nil {
		t.Fatalf("deliverQueryCommand() error = %v", err)
	}
	if injected != command {
		t.Fatalf("injected command = %q, want original %q", injected, command)
	}
}

func TestAskDenialRemembersExactCommand(t *testing.T) {
	t.Parallel()
	const command = "git push --force origin main"
	remembered := ""
	delivery := queryDelivery{
		confirm:        func(context.Context, *domain.Command) (bool, error) { return false, nil },
		rememberDenied: func(_ context.Context, value string) error { remembered = value; return nil },
	}
	err := authorizeQueryExecution(t.Context(), delivery, &domain.Command{Command: command}, security.DecisionAsk)
	if err == nil || remembered != command {
		t.Fatalf("authorizeQueryExecution() = %v, remembered %q", err, remembered)
	}
}

func TestRememberDeniedQueryCommandPersistsExactCommandOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	zevDir := filepath.Join(home, ".zev")
	if err := os.Mkdir(zevDir, 0o700); err != nil {
		t.Fatal(err)
	}
	const command = "git push --force origin main"
	if err := rememberDeniedQueryCommand(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if err := rememberDeniedQueryCommand(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Security.DeniedCommands, []string{command}) {
		t.Fatalf("persisted denied commands = %q", cfg.Security.DeniedCommands)
	}
}
