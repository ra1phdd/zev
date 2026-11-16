package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestQueryDeliveryOutcomeSurvivesWarningFailure(t *testing.T) {
	t.Parallel()
	var outcome historyservice.SelectionOutcome
	outputErr := errors.New("warning unavailable")
	err := deliverQueryCommand(t.Context(), queryDelivery{command: &domain.Command{Command: "true", IsDangerous: true, DangerousExplanation: "warning"}, mode: executionModeExecute, out: io.Discard, errOut: failingOutput{err: outputErr}, logger: setupLogger(0), outcome: &outcome})
	if !errors.Is(err, outputErr) {
		t.Fatalf("error %v", err)
	}
	if outcome.DeliveryOutcome != "executed" || outcome.ExecutionExitCode == nil || *outcome.ExecutionExitCode != 0 || outcome.ExecutedAt == nil {
		t.Fatalf("outcome %+v", outcome)
	}
}

func TestCancelledDeliveryNeverClaimsExecution(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var outcome historyservice.SelectionOutcome
	err := deliverQueryCommand(ctx, queryDelivery{command: &domain.Command{Command: "true"}, mode: executionModeExecute, out: io.Discard, errOut: io.Discard, logger: setupLogger(0), outcome: &outcome})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v", err)
	}
	if outcome.ExecutedAt != nil || outcome.ExecutionExitCode != nil || outcome.DeliveryOutcome == "executed" {
		t.Fatalf("false execution %+v", outcome)
	}
}
