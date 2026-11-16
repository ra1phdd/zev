package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestConversationLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	start := service.StartConversationInput{Conversation: service.Conversation{ID: "c1"}, InitialMessage: service.Message{ID: "m0", Role: userRole, Kind: initialPromptKind, Content: "initial token"}}
	if _, err = s.StartConversation(ctx, start); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		m := service.Message{ID: fmt.Sprint("m", i), ConversationID: "c1", Role: userRole, Kind: refinementKind, Content: fmt.Sprint("refine", i)}
		got, e := s.AppendMessage(ctx, service.AppendMessageInput{Message: m})
		if e != nil || got.Sequence != int64(i) {
			t.Fatalf("append: %+v %v", got, e)
		}
		retry, e := s.AppendMessage(ctx, service.AppendMessageInput{Message: m})
		if e != nil || retry.Sequence != got.Sequence {
			t.Fatalf("retry: %+v %v", retry, e)
		}
		m.Content = "changed"
		if _, e = s.AppendMessage(ctx, service.AppendMessageInput{Message: m}); !errors.Is(e, service.ErrConflict) {
			t.Fatalf("conflict: %v", e)
		}
	}
	input := service.RecordResponseInput{Message: service.Message{ID: "a1", Role: assistantRole, Kind: responseKind, Content: "model output"}, Response: service.Response{ID: "r1", ConversationID: "c1", Provider: "p", Model: modelFixture, PayloadJSON: `{"ok":true}`, IsValid: true, Commands: []service.Command{{ID: firstCommandFixture, Command: "echo hello"}, {ID: "cmd2", Command: "ls"}}}}
	r, err := s.RecordResponse(ctx, input)
	if err != nil || len(r.Commands) != 2 || r.Commands[1].Ordinal != 1 {
		t.Fatalf("response: %+v %v", r, err)
	}
	if _, err = s.RecordResponse(ctx, input); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err = s.RecordSelection(ctx, service.RecordSelectionInput{ConversationID: "wrong", CommandID: firstCommandFixture}); !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("cross conversation: %v", err)
	}
	if err = s.RecordSelection(ctx, service.RecordSelectionInput{ConversationID: "c1", CommandID: firstCommandFixture}); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteConversation(ctx, service.CompleteConversationInput{ConversationID: "c1", Status: completedStatus}); err != nil {
		t.Fatal(err)
	}
	view, err := s.GetConversation(ctx, "c1")
	if err != nil || len(view.Messages) != 5 || len(view.Responses) != 1 || view.Conversation.SelectedCommandID != firstCommandFixture {
		t.Fatalf("view: %+v %v", view, err)
	}
	var found int
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM history_fts WHERE history_fts MATCH ?`, "initial").Scan(&found); err != nil || found != 1 {
		t.Fatalf("fts count=%d err=%v", found, err)
	}
	if err = s.DeleteConversation(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"messages", "responses", "commands", "history_documents", "history_fts"} {
		var n int
		if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s: %d %v", table, n, err)
		}
	}
}

func TestNoRotationAndPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	for i := range 105 {
		id := fmt.Sprintf("%03d", i)
		_, err = s.StartConversation(ctx, service.StartConversationInput{Conversation: service.Conversation{ID: id}, InitialMessage: service.Message{ID: "m" + id, Role: userRole, Kind: initialPromptKind, Content: "hello"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.ListConversations(ctx, service.ListRequest{Limit: 100})
	if err != nil || len(page) != 100 {
		t.Fatalf("first page: %d %v", len(page), err)
	}
	last := page[len(page)-1]
	page, err = s.ListConversations(ctx, service.ListRequest{Limit: 100, BeforeCreatedAt: last.CreatedAt, BeforeID: last.ID})
	if err != nil || len(page) != 5 {
		t.Fatalf("second page: %d %v", len(page), err)
	}
}

func TestPragmasAndAtomicResponseFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	for name, want := range map[string]string{"foreign_keys": "1", "journal_mode": "wal", "busy_timeout": "5000"} {
		var got string
		if err = s.db.QueryRowContext(ctx, "PRAGMA "+name).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: %s %v", name, got, err)
		}
	}
	_, err = s.StartConversation(ctx, service.StartConversationInput{Conversation: service.Conversation{ID: "c"}, InitialMessage: service.Message{ID: "m", Role: userRole, Kind: initialPromptKind, Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordResponse(ctx, service.RecordResponseInput{Message: service.Message{ID: "a", Role: assistantRole, Kind: responseKind}, Response: service.Response{ID: "r", ConversationID: "c", PayloadJSON: "not-json"}})
	if err == nil {
		t.Fatal("invalid json accepted")
	}
	if _, err = s.GetMessage(ctx, "a"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("partial assistant message: %v", err)
	}
}
