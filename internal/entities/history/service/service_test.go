package service_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/history/repository"
	history "github.com/ra1phdd/zev/internal/entities/history/service"
	historymigration "github.com/ra1phdd/zev/internal/migrations/history"
)

const (
	historyInitialPromptKind = "initial_prompt"
	historyRefinementKind    = "refinement"
	historyIncludeHidden     = " include hidden "
	historyAssistantRole     = "assistant"
	historySystemRole        = "system"
	historyUserRole          = "user"
	historyModel             = "model"
	historyListFiles         = "list files"
	historyRequest           = "request"
	historyChosen            = "chosen"
	historyFileContents      = "file contents"
	historyCommandError      = "command_error"
	historyExitStatus        = "exit status 1"
	historyConversationID    = "conversation-1"
	historyListCommand       = "ls -la"
	historyValidPayload      = `{"isValid":true}`
)

func openHistoryStore(t *testing.T) *repository.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private", "history.db")
	if err := historymigration.Migrate(t.Context(), path, true); err != nil {
		t.Fatal(err)
	}
	store, err := repository.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close history store: %v", err)
		}
	})
	return store
}

func TestLifecycleCanCancelAfterInitialPrompt(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	lifecycle := history.NewLifecycle(store)
	if err := lifecycle.Start(t.Context(), historyListFiles, "openai", "model"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Complete(t.Context(), "cancelled"); err != nil {
		t.Fatal(err)
	}

	conversations, err := store.ListConversations(t.Context(), history.ListRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].Status != "cancelled" {
		t.Fatalf("conversations = %+v, want one cancelled conversation", conversations)
	}
	view, err := store.GetConversation(t.Context(), conversations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Messages) != 1 || view.Messages[0].Content != historyListFiles {
		t.Fatalf("messages = %+v, want only the initial prompt", view.Messages)
	}
	if len(view.Responses) != 0 {
		t.Fatalf("responses = %+v, want none after cancellation", view.Responses)
	}
}

func TestLifecyclePersistsRefinementBeforeResponse(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	lifecycle := history.NewLifecycle(store)
	if err := lifecycle.Start(t.Context(), historyListFiles, "openai", "model"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Refine(t.Context(), "include hidden files"); err != nil {
		t.Fatal(err)
	}
	response := &domain.OptionsResponse{
		IsValid: true,
		Commands: []domain.Command{{
			Command:          historyListCommand,
			ShortExplanation: "List files",
		}},
	}
	if err := lifecycle.Response(t.Context(), response); err != nil {
		t.Fatal(err)
	}
	conversationID, commandID, ok := lifecycle.SelectedCommandReference(0)
	if !ok || conversationID == "" || commandID == "" {
		t.Fatalf("selected command reference = (%q, %q, %t)", conversationID, commandID, ok)
	}

	conversations, err := store.ListConversations(t.Context(), history.ListRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.GetConversation(t.Context(), conversations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Messages) != 3 {
		t.Fatalf("messages = %+v, want initial, refinement, and response", view.Messages)
	}
	if view.Messages[0].Kind != historyInitialPromptKind || view.Messages[1].Kind != "refinement" || view.Messages[1].Content != "include hidden files" {
		t.Fatalf("message order/content = %+v", view.Messages)
	}
	if len(view.Responses) != 1 || len(view.Responses[0].Commands) != 1 || view.Responses[0].Commands[0].Command != historyListCommand {
		t.Fatalf("responses = %+v", view.Responses)
	}
}

func TestLifecycleRejectsSelectionBeforeResponse(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	lifecycle := history.NewLifecycle(store)
	if err := lifecycle.Start(t.Context(), historyListFiles, "openai", "model"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Select(t.Context(), history.SelectionOutcome{Index: 0}); !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("Select error = %v, want history.ErrInvalid", err)
	}
}
