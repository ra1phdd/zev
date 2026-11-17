package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	historyrepo "github.com/ra1phdd/zev/internal/entities/history/repository"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/pkg/tui/list"
)

func TestQueryPersistsRefinementAndFullResponses(t *testing.T) {
	t.Parallel()
	store, err := historyrepo.Open(t.Context(), filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	provider := &queryFlowProvider{results: []queryProviderResult{
		{response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "ls", ShortExplanation: "list files"}}}},
		{response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "pwd", ShortExplanation: "working directory"}}}},
	}}
	s := newTestQuerySession(t, provider)
	s.persistence = historyservice.NewLifecycle(store)
	s.provider, s.model = provider.Name(), provider.Model()
	if err := s.start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if ready, err := s.acceptResponse(); err != nil || !ready {
		t.Fatalf("accept: %v %v", ready, err)
	}
	s.input = func(string) (string, error) { return "  exact refinement\n", nil }
	s.selectItem = func(options list.SelectOptions) (list.Item, error) { return options.Items[len(options.Items)-1], nil }
	if ready, err := s.refine(t.Context()); err != nil || !ready {
		t.Fatalf("refine: %v %v", ready, err)
	}
	conversations, err := store.ListConversations(t.Context(), historyservice.ListRequest{Limit: 10})
	if err != nil || len(conversations) != 1 {
		t.Fatalf("conversations: %+v %v", conversations, err)
	}
	view, err := store.GetConversation(t.Context(), conversations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Messages) != 4 || view.Messages[0].Content != queryTestPrompt || view.Messages[2].Content != "  exact refinement\n" {
		t.Fatalf("messages: %+v", view.Messages)
	}
	if len(view.Responses) != 2 || view.Responses[0].ID == "" || view.Responses[1].ID == "" {
		t.Fatalf("responses: %+v", view.Responses)
	}
	var response domain.OptionsResponse
	if err := json.Unmarshal([]byte(view.Responses[1].PayloadJSON), &response); err != nil || response.Commands[0].ShortExplanation != "working directory" {
		t.Fatalf("payload: %+v %v", response, err)
	}
	if len(provider.histories) != 2 || len(provider.histories[1].Messages) != 3 {
		t.Fatalf("provider history: %+v", provider.histories)
	}
}

func TestQueryFailureAndCancellationKeepAlreadyStoredMessages(t *testing.T) {
	t.Parallel()
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: historyStatusFailed, true: "cancelled"}[cancelled], func(t *testing.T) {
			t.Parallel()
			store, err := historyrepo.Open(t.Context(), filepath.Join(t.TempDir(), "private", "history.db"))
			if err != nil {
				t.Fatal(err)
			}
			providerErr := errors.New("provider unavailable")
			if cancelled {
				providerErr = context.Canceled
			}
			provider := &queryFlowProvider{results: []queryProviderResult{{err: providerErr}}}
			s := newTestQuerySession(t, provider)
			s.persistence = historyservice.NewLifecycle(store)
			resultErr := s.start(t.Context())
			if !errors.Is(resultErr, providerErr) {
				t.Fatalf("provider error lost: %v", resultErr)
			}
			conversations, err := store.ListConversations(t.Context(), historyservice.ListRequest{Limit: 10})
			if err != nil || len(conversations) != 1 {
				t.Fatalf("conversations %+v, %v", conversations, err)
			}
			id := conversations[0].ID
			status := "failed"
			if cancelled {
				status = "cancelled"
			}
			if err := s.persistence.Complete(t.Context(), status); err != nil {
				t.Fatal(err)
			}
			view, err := store.GetConversation(t.Context(), id)
			if err != nil || len(view.Messages) != 1 || view.Messages[0].Content != queryTestPrompt || view.Conversation.Status != status {
				t.Fatalf("partial history %+v, %v", view, err)
			}
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestHistoryWriteFailureWarnsWithoutReplacingProviderFailure(t *testing.T) {
	t.Parallel()
	store, err := historyrepo.Open(t.Context(), filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	providerErr := errors.New("provider unavailable")
	s := newTestQuerySession(t, &queryFlowProvider{results: []queryProviderResult{{err: providerErr}}})
	s.persistence = historyservice.NewLifecycle(store)
	var diagnostics bytes.Buffer
	s.cmd.SetErr(&diagnostics)
	if err := s.start(t.Context()); !errors.Is(err, providerErr) {
		t.Fatalf("provider error replaced: %v", err)
	}
	if diagnostics.Len() == 0 {
		t.Fatal("history write failure was silent")
	}
}
