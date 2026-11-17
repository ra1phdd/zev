package cli

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestQueryDeliveryRejectsInvalidCommands(t *testing.T) {
	t.Parallel()
	for _, command := range []*domain.Command{nil, {Command: ""}, {Command: "echo \x00bad"}, {Command: "echo \x1b[31m"}} {
		err := deliverQueryCommand(t.Context(), queryDelivery{command: command, mode: executionModeExecute, out: io.Discard, errOut: io.Discard, logger: slog.New(slog.DiscardHandler)})
		if !errors.Is(err, domain.ErrInvalidOptionsResponse) {
			t.Fatalf("error=%v; invalid command reached delivery", err)
		}
	}
}

func TestQueryAcceptRejectsNilResponse(t *testing.T) {
	t.Parallel()
	s := newTestQuerySession(t, &queryFlowProvider{})
	if ready, err := s.acceptResponse(); ready || !errors.Is(err, domain.ErrInvalidOptionsResponse) {
		t.Fatalf("ready=%v error=%v", ready, err)
	}
}
