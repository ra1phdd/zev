package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/security"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	"github.com/ra1phdd/zev/pkg/terminal"
)

func TestExecuteDeliveryUsesSameProviderReviewForUnknownCommand(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: io.Discard}
	var outcome historyservice.SelectionOutcome
	reviewed := false
	err := deliverQueryCommand(t.Context(), queryDelivery{
		command: &domain.Command{Command: `printf '%s' "$HOME"`, ShortExplanation: "Print the home directory"},
		mode:    executionModeExecute, securityMode: security.ModeNormal,
		userRequest: "Show my home directory",
		reviewCommand: func(_ context.Context, command, request, explanation string) (queryservice.CommandSafetyReview, error) {
			reviewed = true
			if command != `printf '%s' "$HOME"` || request != "Show my home directory" || explanation != "Print the home directory" {
				t.Fatalf("review input = %q, %q, %q", command, request, explanation)
			}
			return queryservice.CommandSafetyReview{Safe: true, Reason: "Reads one environment value."}, nil
		},
		out: &stdout, errOut: io.Discard, logger: setupLogger(0), runner: &runner, outcome: &outcome,
	})
	if err != nil || !reviewed || stdout.Len() == 0 || outcome.DeliveryOutcome != queryDeliveryExecuted {
		t.Fatalf("reviewed unknown command did not execute: err=%v reviewed=%t output=%q outcome=%+v", err, reviewed, stdout.String(), outcome)
	}
}

func TestExecuteDeliveryAutomaticallyRetriesAfterCommandFailure(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	runner := terminal.Runner{Stdout: &stdout, Stderr: &stderr}
	var outcome historyservice.SelectionOutcome
	recoveryCalls := 0
	err := executeQueryAttempts(t.Context(), queryDelivery{
		command: &domain.Command{Command: `printf initial-output; exit 1`, ShortExplanation: "Fail once"},
		mode:    executionModeExecute, securityMode: security.ModeNormal, userRequest: "Run the command",
		reviewCommand: func(context.Context, string, string, string) (queryservice.CommandSafetyReview, error) {
			return queryservice.CommandSafetyReview{Safe: true, Reason: "The command is allowed for this test."}, nil
		},
		out: &stdout, errOut: &stderr, logger: setupLogger(0), runner: &runner, outcome: &outcome,
	}, func(_ context.Context, command *domain.Command, commandErr error, output runOutput) (*domain.Command, error) {
		recoveryCalls++
		if recoveryCalls == 1 && (!strings.Contains(commandErr.Error(), "exit status 1") || output.stdout != "initial-output") {
			t.Fatalf("first failure context = command=%q error=%v output=%+v", command.Command, commandErr, output)
		}
		if recoveryCalls == 1 {
			return &domain.Command{Command: `printf retry; exit 1`, ShortExplanation: "Try a corrected command"}, nil
		}
		if recoveryCalls == 2 && output.stdout != "retry" {
			t.Fatalf("second failure output = %+v; want retry", output)
		}
		return &domain.Command{Command: "printf recovered", ShortExplanation: "Recover from the failure"}, nil
	})
	if err != nil || recoveryCalls != 2 || outcome.DeliveryOutcome != queryDeliveryExecuted || !strings.Contains(stdout.String(), "initial-outputretryrecovered") {
		t.Fatalf("automatic retries = err=%v recovery_calls=%d output=%q outcome=%+v", err, recoveryCalls, stdout.String(), outcome)
	}
}
