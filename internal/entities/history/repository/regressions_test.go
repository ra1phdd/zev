package repository

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const (
	providerFixture     = "provider"
	echoFixture         = "echo"
	conversationFixture = "conversation"
	initialFixture      = "initial"
)

func regressionStore(t *testing.T) *Store {
	t.Helper()
	s, err := openTestStore(t.Context(), t, filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func responseInput(conversationID, responseID, messageID string, created time.Time, command service.Command) service.RecordResponseInput {
	return service.RecordResponseInput{
		Message: service.Message{ID: messageID, Role: assistantRole, Kind: responseKind, Content: "response", CreatedAt: created},
		Response: service.Response{
			ID: responseID, ConversationID: conversationID, Provider: providerFixture, Model: "model",
			PayloadJSON: `{}`, IsValid: true, CreatedAt: created, Commands: []service.Command{command},
		},
	}
}

func TestRebuildFTSUsesCommandResponseTimestamp(t *testing.T) {
	t.Parallel()
	s := regressionStore(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	responded := created.Add(time.Hour)
	if _, err := s.StartConversation(t.Context(), service.StartConversationInput{
		Conversation:   service.Conversation{ID: conversationFixture, CreatedAt: created},
		InitialMessage: service.Message{ID: initialFixture, Role: userRole, Kind: initialPromptKind, Content: initialFixture, CreatedAt: created},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordResponse(t.Context(), responseInput(conversationFixture, "response", "assistant", responded, service.Command{ID: commandDocument, Command: "find timestamp"})); err != nil {
		t.Fatal(err)
	}
	want := service.SearchRequest{Query: "timestamp", Type: commandDocument, Fuzzy: fuzzyOff, From: responded, To: responded.Add(time.Second)}
	if result, err := s.LexicalSearch(t.Context(), want); err != nil || len(result.Hits) != 1 {
		t.Fatalf("before rebuild: %+v, %v", result, err)
	}
	if err := s.RebuildFTS(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := s.LexicalSearch(t.Context(), want)
	if err != nil || len(result.Hits) != 1 || !result.Hits[0].CreatedAt.Equal(responded) {
		t.Fatalf("after rebuild: %+v, %v", result, err)
	}
}

func TestStartConversationRetryUsesImmutableCreationLedger(t *testing.T) {
	t.Parallel()
	s := regressionStore(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	input := service.StartConversationInput{
		Conversation:   service.Conversation{ID: conversationFixture, CreatedAt: created, Provider: "initial-provider", Model: "initial-model"},
		InitialMessage: service.Message{ID: initialFixture, Role: userRole, Kind: initialPromptKind, Content: "immutable"},
	}
	if _, err := s.StartConversation(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordResponse(t.Context(), responseInput(conversationFixture, "response", "assistant", created.Add(time.Hour), service.Command{ID: commandDocument, Command: echoFixture})); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartConversation(t.Context(), input); err != nil {
		t.Fatalf("retry after response: %v", err)
	}
	changed := input
	changed.Conversation.Provider = "different-provider"
	if _, err := s.StartConversation(t.Context(), changed); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("changed creation payload error = %v", err)
	}
}

func TestRecordSelectionDoesNotRegressExecutedCommand(t *testing.T) {
	t.Parallel()
	s := regressionStore(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.StartConversation(t.Context(), service.StartConversationInput{
		Conversation:   service.Conversation{ID: conversationFixture, CreatedAt: created},
		InitialMessage: service.Message{ID: initialFixture, Role: userRole, Kind: initialPromptKind, Content: "select"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordResponse(t.Context(), responseInput(conversationFixture, "response", "assistant", created.Add(time.Hour), service.Command{ID: commandDocument, Command: echoFixture})); err != nil {
		t.Fatal(err)
	}
	selected := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	if err := s.RecordSelection(t.Context(), service.RecordSelectionInput{ConversationID: conversationFixture, CommandID: commandDocument, SelectedAt: selected, DeliveryOutcome: "selected"}); err != nil {
		t.Fatal(err)
	}
	executed := selected.Add(time.Minute)
	exitCode := 0
	if err := s.RecordSelection(t.Context(), service.RecordSelectionInput{ConversationID: conversationFixture, CommandID: commandDocument, SelectedAt: executed, ExecutedAt: &executed, ExecutionExitCode: &exitCode, DeliveryOutcome: executedDelivery}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSelection(t.Context(), service.RecordSelectionInput{ConversationID: conversationFixture, CommandID: commandDocument, SelectedAt: selected, DeliveryOutcome: "selected"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.GetConversation(t.Context(), conversationFixture)
	if err != nil {
		t.Fatal(err)
	}
	command := view.Responses[0].Commands[0]
	if command.DeliveryOutcome != "executed" || command.ExecutedAt == nil || !command.ExecutedAt.Equal(executed) || command.ExecutionExitCode == nil || *command.ExecutionExitCode != 0 {
		t.Fatalf("regressed command: %+v", command)
	}
	if command.SelectedAt == nil || !command.SelectedAt.Equal(selected) {
		t.Fatalf("selection timestamp changed: %+v", command.SelectedAt)
	}
}

func TestTypoMorphologyVerificationUsesExpandedStems(t *testing.T) {
	t.Parallel()
	s := regressionStore(t)
	for _, text := range []string{"connect", "connected", "connecting"} {
		if _, err := s.StartConversation(t.Context(), service.StartConversationInput{
			Conversation:   service.Conversation{ID: text},
			InitialMessage: service.Message{ID: "message-" + text, Role: userRole, Kind: initialPromptKind, Content: text},
		}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.LexicalSearch(t.Context(), service.SearchRequest{Query: "connceting", Fuzzy: fuzzyAlways, Limit: 10})
	if err != nil || len(result.Hits) != 3 {
		t.Fatalf("typo morphology search: %+v, %v", result, err)
	}
}
