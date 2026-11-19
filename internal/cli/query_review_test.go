package cli

import (
	"bytes"
	"context"
	"io"
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
