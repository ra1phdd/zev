package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const (
	lastCommandFixture = "echo latest"
	lastCommandEarlier = "earlier"
	lastCommandLatest  = "latest"
	executedDelivery   = "executed"
	cancelledFixture   = "cancelled"
	printedDelivery    = "printed"
)

func TestLastUsedCommand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, status, outcome, command string
		selected                       bool
		exit                           *int
		executionError                 string
		wantLatest                     bool
	}{
		{name: printedDelivery, status: completedStatus, outcome: printedDelivery, command: lastCommandFixture, selected: true, wantLatest: true},
		{name: "injected", status: completedStatus, outcome: "injected", command: lastCommandFixture, selected: true, wantLatest: true},
		{name: executedDelivery, status: completedStatus, outcome: executedDelivery, command: lastCommandFixture, selected: true, exit: new(0), wantLatest: true},
		{name: cancelledFixture, status: cancelledFixture, outcome: printedDelivery, command: lastCommandFixture, selected: true},
		{name: "failed conversation", status: failedStatus, outcome: printedDelivery, command: lastCommandFixture, selected: true},
		{name: startedStatus, status: startedStatus, outcome: printedDelivery, command: lastCommandFixture, selected: true},
		{name: "unselected", status: completedStatus, command: lastCommandFixture},
		{name: "selected only", status: completedStatus, outcome: selectedOutcome, command: lastCommandFixture, selected: true},
		{name: "unknown", status: completedStatus, outcome: unknownOutcome, command: lastCommandFixture, selected: true},
		{name: "failed delivery", status: completedStatus, outcome: failedStatus, command: lastCommandFixture, selected: true, exit: new(1)},
		{name: "nonzero execution", status: completedStatus, outcome: executedDelivery, command: lastCommandFixture, selected: true, exit: new(1)},
		{name: "execution error", status: completedStatus, outcome: executedDelivery, command: lastCommandFixture, selected: true, exit: new(0), executionError: failedStatus},
		{name: "empty command", status: completedStatus, outcome: printedDelivery, command: " \t\n", selected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := regressionStore(t)
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, id := range []string{lastCommandEarlier, lastCommandLatest} {
				// Creation order differs from selection order.
				created := at.Add(time.Hour)
				if id == lastCommandLatest {
					created = at
				}
				if _, err := s.StartConversation(t.Context(), service.StartConversationInput{
					Conversation:   service.Conversation{ID: id, CreatedAt: created},
					InitialMessage: service.Message{ID: id + "-user", Role: userRole, Kind: initialPromptKind, Content: "fixture"},
				}); err != nil {
					t.Fatal(err)
				}
				text := "echo earlier"
				if id == lastCommandLatest {
					text = tc.command
				}
				in := responseInput(id, id+"-response", id+"-assistant", created, service.Command{ID: id + "-command", Command: text})
				in.Response.Commands = append(in.Response.Commands, service.Command{ID: id + "-unselected", Command: "echo wrong alternative"})
				if _, err := s.RecordResponse(t.Context(), in); err != nil {
					t.Fatal(err)
				}
				selected, status, outcome := true, completedStatus, printedDelivery
				var exit *int
				var executionError string
				if id == lastCommandLatest {
					selected, status, outcome = tc.selected, tc.status, tc.outcome
					exit = tc.exit
					executionError = tc.executionError
				}
				if selected {
					selection := service.RecordSelectionInput{ConversationID: id, CommandID: id + "-command", SelectedAt: at, DeliveryOutcome: outcome, ExecutionExitCode: exit, ExecutionError: executionError}
					if id == lastCommandLatest {
						selection.SelectedAt = at.Add(2 * time.Hour)
					}
					if outcome == executedDelivery {
						selection.ExecutedAt = &selection.SelectedAt
					}
					if err := s.RecordSelection(t.Context(), selection); err != nil {
						t.Fatal(err)
					}
				}
				if status != startedStatus {
					if err := s.CompleteConversation(t.Context(), service.CompleteConversationInput{ConversationID: id, Status: status}); err != nil {
						t.Fatal(err)
					}
				}
			}
			got, err := s.LastUsedCommand(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			want := "earlier-command"
			if tc.wantLatest {
				want = "latest-command"
			}
			if got.ID != want {
				t.Fatalf("command=%+v, want %s", got, want)
			}
		})
	}
}

func TestLastUsedCommandEmpty(t *testing.T) {
	t.Parallel()
	s := regressionStore(t)
	if _, err := s.LastUsedCommand(t.Context()); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("error=%v, want not found", err)
	}
}
