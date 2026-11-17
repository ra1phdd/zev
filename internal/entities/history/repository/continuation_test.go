package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const continuationEarlierID = "earlier"

func TestLastConversationFiltersTerminalQueries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, status, kind, prompt, refinement string
		wantLatest                             bool
	}{
		{name: completedStatus, status: completedStatus, prompt: lastCommandLatest, wantLatest: true},
		{name: "failed provider without response", status: failedStatus, prompt: lastCommandLatest, wantLatest: true},
		{name: "cancelled", status: "cancelled", prompt: lastCommandLatest},
		{name: startedStatus, status: startedStatus, prompt: lastCommandLatest},
		{name: "not query", status: completedStatus, kind: "legacy", prompt: lastCommandLatest},
		{name: "blank prompt", status: completedStatus, prompt: " 	\r\n"},
		{name: "unicode blank", status: completedStatus, prompt: "\u2003\u00a0\u3000"},
		{name: "refinement only", status: failedStatus, prompt: " 	", refinement: "usable clarification", wantLatest: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := regressionStore(t)
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, id := range []string{continuationEarlierID, lastCommandLatest} {
				prompt, kind, status := "earlier request", "query", completedStatus
				if id == lastCommandLatest {
					prompt, kind, status = tc.prompt, tc.kind, tc.status
				}
				if _, err := s.StartConversation(t.Context(), service.StartConversationInput{
					Conversation:   service.Conversation{ID: id, Kind: kind, CreatedAt: at},
					InitialMessage: service.Message{ID: id + "-user", Role: userRole, Kind: initialPromptKind, Content: prompt},
				}); err != nil {
					t.Fatal(err)
				}
				if id == lastCommandLatest && tc.refinement != "" {
					if _, err := s.AppendMessage(t.Context(), service.AppendMessageInput{Message: service.Message{ID: "refinement", ConversationID: id, Role: userRole, Kind: refinementKind, Content: tc.refinement}}); err != nil {
						t.Fatal(err)
					}
				}
				updated := at.Add(time.Hour)
				if id == lastCommandLatest {
					updated = at.Add(2 * time.Hour)
				}
				if status != startedStatus {
					if err := s.CompleteConversation(t.Context(), service.CompleteConversationInput{ConversationID: id, Status: status, CompletedAt: updated}); err != nil {
						t.Fatal(err)
					}
				}
			}
			view, err := s.LastConversation(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			want := continuationEarlierID
			if tc.wantLatest {
				want = lastCommandLatest
			}
			if view.Conversation.ID != want {
				t.Fatalf("conversation = %s, want %s", view.Conversation.ID, want)
			}
		})
	}
}

func TestLastConversationEmpty(t *testing.T) {
	t.Parallel()
	s := regressionStore(t)
	if _, err := s.LastConversation(t.Context()); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestLastConversationOrdersByUpdatedAtThenID(t *testing.T) {
	t.Parallel()
	s := regressionStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"a", "b", "c"} {
		if _, err := s.StartConversation(t.Context(), service.StartConversationInput{
			Conversation:   service.Conversation{ID: id, CreatedAt: at.Add(time.Duration(i) * time.Hour)},
			InitialMessage: service.Message{ID: id + "-user", Role: userRole, Kind: initialPromptKind, Content: id},
		}); err != nil {
			t.Fatal(err)
		}
		updated := at.Add(4 * time.Hour)
		if id == "c" {
			updated = at.Add(3 * time.Hour)
		}
		if err := s.CompleteConversation(t.Context(), service.CompleteConversationInput{ConversationID: id, Status: completedStatus, CompletedAt: updated}); err != nil {
			t.Fatal(err)
		}
	}
	view, err := s.LastConversation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view.Conversation.ID != "b" {
		t.Fatalf("latest ID = %s, want b", view.Conversation.ID)
	}
}
