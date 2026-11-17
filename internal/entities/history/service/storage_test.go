package service_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestSQLiteLifecycleOperationsAreIdempotent(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	ctx := t.Context()
	start := service.StartConversationInput{
		Conversation:   service.Conversation{ID: historyConversationID, Kind: "query", Provider: "openai", Model: "model"},
		InitialMessage: service.Message{ID: "message-1", Role: historyUserRole, Kind: historyInitialPromptKind, Content: historyListFiles},
	}
	if _, err := store.StartConversation(ctx, start); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartConversation(ctx, start); err != nil {
		t.Fatalf("retry StartConversation: %v", err)
	}

	refinement := service.Message{ID: "message-2", ConversationID: historyConversationID, Role: historyUserRole, Kind: "refinement", Content: "include hidden files"}
	first, err := store.AppendMessage(ctx, service.AppendMessageInput{Message: refinement})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendMessage(ctx, service.AppendMessageInput{Message: refinement})
	if err != nil {
		t.Fatalf("retry AppendMessage: %v", err)
	}
	if first.Sequence != second.Sequence {
		t.Fatalf("retry sequence = %d, want %d", second.Sequence, first.Sequence)
	}

	response := service.RecordResponseInput{
		Message: service.Message{ID: "message-3", Role: "assistant", Kind: historyResponseKind, Content: historyValidPayload},
		Response: service.Response{
			ID: "response-1", ConversationID: historyConversationID, Provider: "openai", Model: "model",
			PayloadJSON: historyValidPayload, IsValid: true,
			Commands: []service.Command{{ID: "command-1", Command: historyListCommand}},
		},
	}
	if _, err := store.RecordResponse(ctx, response); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordResponse(ctx, response); err != nil {
		t.Fatalf("retry RecordResponse: %v", err)
	}

	view, err := store.GetConversation(ctx, historyConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Messages) != 3 || len(view.Responses) != 1 || len(view.Responses[0].Commands) != 1 {
		t.Fatalf("idempotent view = %+v, want three messages and one response", view)
	}
}

func TestSQLiteLifecycleRetainsMoreThanLegacyHistoryLimit(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	ctx := t.Context()
	for i := range 105 {
		id := fmt.Sprintf("conversation-%03d", i)
		_, err := store.StartConversation(ctx, service.StartConversationInput{
			Conversation:   service.Conversation{ID: id},
			InitialMessage: service.Message{ID: "message-" + id, Role: historyUserRole, Kind: historyInitialPromptKind, Content: id},
		})
		if err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
	}

	page, err := store.ListConversations(ctx, service.ListRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 100 {
		t.Fatalf("first page contains %d conversations, want 100", len(page))
	}
	last := page[len(page)-1]
	page, err = store.ListConversations(ctx, service.ListRequest{Limit: 100, BeforeCreatedAt: last.CreatedAt, BeforeID: last.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 5 {
		t.Fatalf("second page contains %d conversations, want 5", len(page))
	}
}

func TestSQLiteLifecycleRejectsCrossConversationSelection(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	ctx := t.Context()
	if _, err := store.StartConversation(ctx, service.StartConversationInput{
		Conversation:   service.Conversation{ID: historyConversationID},
		InitialMessage: service.Message{ID: "message-1", Role: historyUserRole, Kind: historyInitialPromptKind, Content: "one"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartConversation(ctx, service.StartConversationInput{
		Conversation:   service.Conversation{ID: "conversation-2"},
		InitialMessage: service.Message{ID: "message-2", Role: historyUserRole, Kind: historyInitialPromptKind, Content: "two"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordResponse(ctx, service.RecordResponseInput{
		Message: service.Message{ID: "assistant-2", Role: "assistant", Kind: historyResponseKind, Content: historyValidPayload},
		Response: service.Response{
			ID: "response-2", ConversationID: "conversation-2", PayloadJSON: historyValidPayload, IsValid: true,
			Commands: []service.Command{{ID: "command-2", Command: "pwd"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSelection(ctx, service.RecordSelectionInput{ConversationID: historyConversationID, CommandID: "command-2"}); !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("cross-conversation selection error = %v, want ErrInvalid", err)
	}
}
