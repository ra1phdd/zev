package service_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	history "github.com/ra1phdd/zev/internal/entities/history/service"
)

const (
	historyResponseKind       = "response"
	historySortBySize         = "sort by size"
	historyPrintedOutcome     = "printed"
	historyProjectContextKind = "project_context"
)

func TestContinuationPreservesInheritedTranscriptTwice(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	var inherited []domain.Message
	for round := range 3 {
		lifecycle := history.NewLifecycle(store)
		prompt := []string{historyListFiles, historyIncludeHidden, historySortBySize}[round]
		if round == 0 {
			if err := lifecycle.Start(t.Context(), prompt, "provider", historyModel); err != nil {
				t.Fatal(err)
			}
		} else if err := lifecycle.StartWithHistory(t.Context(), history.StartHistoryInput{
			Prompt: prompt, Provider: "provider", Model: historyModel, ConversationHistory: inherited,
		}); err != nil {
			t.Fatal(err)
		}
		if err := lifecycle.Refine(t.Context(), "first clarification"); err != nil {
			t.Fatal(err)
		}
		response := &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "ls -a"}}}
		if err := lifecycle.Response(t.Context(), response); err != nil {
			t.Fatal(err)
		}
		if err := lifecycle.Refine(t.Context(), "second clarification"); err != nil {
			t.Fatal(err)
		}
		if err := lifecycle.Response(t.Context(), response); err != nil {
			t.Fatal(err)
		}
		if err := lifecycle.Complete(t.Context(), "completed"); err != nil {
			t.Fatal(err)
		}
		view, err := store.LastConversation(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if round == 0 && view.Conversation.MetadataJSON != "" {
			t.Fatalf("Start metadata = %q, want absent", view.Conversation.MetadataJSON)
		}
		if len(view.Messages) != 5 || view.Messages[0].Content != prompt {
			t.Fatalf("current messages = %#v, want exact prompt and current turns only", view.Messages)
		}
		want := append(append([]domain.Message(nil), inherited...),
			domain.Message{Role: historyUserRole, Content: prompt},
			domain.Message{Role: historyUserRole, Content: "first clarification"},
			domain.Message{Role: historyAssistantRole, Kind: historyResponseKind, Content: response.CompactJSON()},
			domain.Message{Role: historyUserRole, Content: "second clarification"},
			domain.Message{Role: historyAssistantRole, Kind: historyResponseKind, Content: response.CompactJSON()},
		)
		got, err := history.RestoreConversation(view)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round %d restored = %#v, want %#v", round, got, want)
		}
		inherited = got
	}
}

func TestRestoreConversationRejectsInvalidTranscript(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, metadata string
	}{
		{"malformed metadata", `{"conversation_history":`},
		{"wrong type", `{"conversation_history":"private-content"}`},
		{"null", `{"conversation_history":null}`},
		{"invalid message", `{"conversation_history":[null]}`},
		{"system role", `{"conversation_history":[{"role":"system","content":"private-content"}]}`},
		{"unknown role", `{"conversation_history":[{"role":"tool","content":"private-content"}]}`},
		{"blank content", `{"conversation_history":[{"role":"user","content":" \t"}]}`},
		{"missing role", `{"conversation_history":[{"content":"private-content"}]}`},
		{"invalid UTF8", "{\"conversation_history\":[{\"role\":\"user\",\"content\":\"" + string([]byte{0xff}) + "\"}]}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := history.RestoreConversation(history.ConversationView{
				Conversation: history.Conversation{MetadataJSON: tc.metadata},
				Messages:     []history.Message{{Role: historyUserRole, Kind: historyInitialPromptKind, Content: "current"}},
			})
			if !errors.Is(err, history.ErrInvalid) || got != nil {
				t.Fatalf("messages = %#v, error = %v; want no messages and invalid", got, err)
			}
			if strings.Contains(err.Error(), "private-content") {
				t.Fatal("error disclosed transcript content")
			}
		})
	}
}

func TestRestoreConversationRequiresUser(t *testing.T) {
	t.Parallel()
	for _, metadata := range []string{"", `{}`, `{"legacy":"metadata"}`, `{"conversation_history":[]}`} {
		if got, err := history.RestoreConversation(history.ConversationView{Conversation: history.Conversation{MetadataJSON: metadata}}); !errors.Is(err, history.ErrInvalid) || got != nil {
			t.Fatalf("metadata %q: messages = %#v, error = %v", metadata, got, err)
		}
	}
}

func TestRestoreConversationTranscriptLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		messages  []domain.Message
		current   string
		wantError bool
	}{
		{name: "byte limit", messages: []domain.Message{{Role: historyUserRole, Content: strings.Repeat("x", 1<<20)}}},
		{name: "too many bytes", messages: []domain.Message{{Role: historyUserRole, Content: strings.Repeat("x", (1<<20)+1)}}, wantError: true},
		{name: "combined bytes", messages: []domain.Message{{Role: historyUserRole, Content: strings.Repeat("x", 1<<20)}}, current: "x", wantError: true},
		{name: "message limit", messages: repeatHistoryMessages(512)},
		{name: "too many messages", messages: repeatHistoryMessages(513), wantError: true},
		{name: "combined messages", messages: repeatHistoryMessages(512), current: "x", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(map[string]any{"conversation_history": tc.messages})
			if err != nil {
				t.Fatal(err)
			}
			view := history.ConversationView{Conversation: history.Conversation{MetadataJSON: string(encoded)}}
			if tc.current != "" {
				view.Messages = []history.Message{{Role: historyUserRole, Kind: historyInitialPromptKind, Content: tc.current}}
			}
			_, err = history.RestoreConversation(view)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError %v", err, tc.wantError)
			}
		})
	}
}

func repeatHistoryMessages(count int) []domain.Message {
	messages := make([]domain.Message, count)
	for i := range messages {
		messages[i] = domain.Message{Role: historyUserRole, Content: "x"}
	}
	return messages
}

func TestRestoreConversationRejectsMalformedResponse(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`not-json-private-content`, `{}`, `{"is_valid":true,"commands":[]}`,
		`{"is_valid":true,"commands":[{"command":""}]}`,
		`{"is_valid":false}`, "{\"is_valid\":true,\"commands\":[{\"command\":\"" + string([]byte{0xff}) + "\"}]}",
	} {
		view := history.ConversationView{
			Messages:  []history.Message{{Role: historyUserRole, Kind: historyInitialPromptKind, Content: historyRequest}, {ID: "a", Sequence: 1, Role: historyAssistantRole, Kind: historyResponseKind}},
			Responses: []history.Response{{MessageID: "a", PayloadJSON: payload}},
		}
		got, err := history.RestoreConversation(view)
		if !errors.Is(err, history.ErrInvalid) || got != nil {
			t.Fatalf("messages = %#v, error = %v; want invalid", got, err)
		}
		if strings.Contains(err.Error(), "private-content") {
			t.Fatal("error disclosed response")
		}
	}
}

func TestRestoreConversationSelectionContext(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, outcome, executionError string
		exit                          *int
		selected                      string
		wantContext                   bool
	}{
		{name: historyPrintedOutcome, outcome: historyPrintedOutcome, selected: historyChosen, wantContext: true},
		{name: "injected", outcome: "injected", selected: historyChosen, wantContext: true},
		{name: "selected only", outcome: "selected", selected: historyChosen, wantContext: true},
		{name: "observed failure", outcome: "failed", executionError: "missing dependency", exit: new(127), selected: historyChosen, wantContext: true},
		{name: "observed success", outcome: "executed", exit: new(0), selected: historyChosen, wantContext: true},
		{name: "unselected", selected: "other"},
		{name: "no selection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := history.ConversationView{
				Conversation: history.Conversation{SelectedCommandID: tc.selected},
				Messages:     []history.Message{{Role: historyUserRole, Kind: historyInitialPromptKind, Content: historyRequest}, {ID: "a", Sequence: 1, Role: historyAssistantRole, Kind: historyResponseKind}},
				Responses:    []history.Response{{MessageID: "a", PayloadJSON: `{"is_valid":true,"commands":[{"command":"echo chosen"}]}`, Commands: []history.Command{{ID: historyChosen, Command: "echo chosen", DeliveryOutcome: tc.outcome, ExecutionError: tc.executionError, ExecutionExitCode: tc.exit}}}},
			}
			messages, err := history.RestoreConversation(view)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantContext {
				if len(messages) != 2 {
					t.Fatalf("unselected command context: %#v", messages)
				}
				return
			}
			if len(messages) != 3 || messages[2].Role != "user" || messages[2].Kind != "command_context" {
				t.Fatalf("messages = %#v", messages)
			}
			content := messages[2].Content
			if !strings.Contains(content, "[COMMAND]\necho chosen\n[/COMMAND]") || !strings.Contains(content, "Observed delivery outcome: "+tc.outcome) {
				t.Fatalf("selection context = %q", content)
			}
			if tc.executionError != "" && !strings.Contains(content, "[COMMAND_ERROR]\n"+tc.executionError+"\n[/COMMAND_ERROR]") {
				t.Fatalf("missing error context: %q", content)
			}
			if tc.exit != nil {
				wantExit := fmt.Sprintf("Observed exit code: %d", *tc.exit)
				if !strings.Contains(content, wantExit) {
					t.Fatalf("missing exit code %q: %q", wantExit, content)
				}
			}
			if strings.Contains(content, "was executed") || strings.Contains(content, "ran successfully") {
				t.Fatalf("unobserved execution claim: %q", content)
			}
		})
	}
}

func TestRestoreConversationSelectionImmediatelyFollowsResponse(t *testing.T) {
	t.Parallel()
	view := history.ConversationView{
		Conversation: history.Conversation{SelectedCommandID: historyChosen},
		Messages:     []history.Message{{Role: historyUserRole, Kind: historyInitialPromptKind, Content: historyRequest}, {ID: "a", Sequence: 1, Role: historyAssistantRole, Kind: historyResponseKind}, {Sequence: 2, Role: historyUserRole, Kind: historyRefinementKind, Content: "later clarification"}},
		Responses:    []history.Response{{MessageID: "a", PayloadJSON: `{"is_valid":true,"commands":[{"command":"echo chosen"}]}`, Commands: []history.Command{{ID: historyChosen, Command: "echo chosen", DeliveryOutcome: historyPrintedOutcome}}}},
	}
	messages, err := history.RestoreConversation(view)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[3].Content != "later clarification" || messages[2].Kind != "command_context" || !strings.Contains(messages[2].Content, "[COMMAND]") {
		t.Fatalf("selection order = %#v", messages)
	}
}

func TestStartWithHistoryRejectsInvalidSnapshot(t *testing.T) {
	t.Parallel()
	for _, messages := range [][]domain.Message{
		{{Role: historySystemRole, Content: "private-content"}},
		{{Role: historyUserRole, Content: " \t"}},
		{{Role: historyUserRole, Content: string([]byte{0xff})}},
		repeatHistoryMessages(513),
		{{Role: historyUserRole, Content: strings.Repeat("x", (1<<20)+1)}},
	} {
		store := openHistoryStore(t)
		err := history.NewLifecycle(store).StartWithHistory(t.Context(), history.StartHistoryInput{Prompt: "current", ConversationHistory: messages})
		if !errors.Is(err, history.ErrInvalid) {
			t.Fatalf("error = %v, want invalid", err)
		}
		conversations, err := store.ListConversations(t.Context(), history.ListRequest{Limit: 10})
		if err != nil || len(conversations) != 0 {
			t.Fatalf("invalid snapshot wrote conversation: %#v, %v", conversations, err)
		}
	}
}

func TestLifecycleAppendPersistsContextKindsWithoutSchemaChanges(t *testing.T) {
	t.Parallel()
	store := openHistoryStore(t)
	lifecycle := history.NewLifecycle(store)
	if err := lifecycle.Append(t.Context(), "context", historyProjectContextKind); !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("before start error = %v", err)
	}
	if err := lifecycle.Start(t.Context(), "request", "provider", historyModel); err != nil {
		t.Fatal(err)
	}
	for _, message := range []domain.Message{
		{Kind: historyProjectContextKind, Content: historyFileContents},
		{Kind: historyCommandError, Content: historyExitStatus},
		{Kind: historyRefinementKind, Content: "clarification"},
	} {
		if err := lifecycle.Append(t.Context(), message.Content, message.Kind); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"", historyResponseKind, "initial_prompt", "unknown"} {
		if err := lifecycle.Append(t.Context(), "not valid", kind); !errors.Is(err, history.ErrInvalid) {
			t.Fatalf("kind %q error = %v", kind, err)
		}
	}
	conversations, err := store.ListConversations(t.Context(), history.ListRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.GetConversation(t.Context(), conversations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Messages) != 4 || view.Messages[1].Content != "file contents" || view.Messages[2].Content != "exit status 1" {
		t.Fatalf("stored context = %#v", view.Messages)
	}
	got, err := history.RestoreConversation(view)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Message{{Role: historyUserRole, Content: historyRequest}, {Role: historyUserRole, Kind: historyProjectContextKind, Content: historyFileContents}, {Role: historyUserRole, Kind: historyCommandError, Content: historyExitStatus}, {Role: historyUserRole, Content: "clarification"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("restored = %#v, want %#v", got, want)
	}
}

func TestContinuationRestoresContextOnlyConversation(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{historyProjectContextKind, "command_error"} {
		t.Run(kind, func(t *testing.T) {
			store := openHistoryStore(t)
			lifecycle := history.NewLifecycle(store)
			if err := lifecycle.Start(t.Context(), "", "provider", historyModel); err != nil {
				t.Fatal(err)
			}
			if err := lifecycle.Append(t.Context(), "provided context", kind); err != nil {
				t.Fatal(err)
			}
			if err := lifecycle.Complete(t.Context(), "failed"); err != nil {
				t.Fatal(err)
			}
			view, err := store.LastConversation(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if view.Messages[0].Content != "" {
				t.Fatal("initial prompt was changed")
			}
			got, err := history.RestoreConversation(view)
			if err != nil {
				t.Fatal(err)
			}
			want := []domain.Message{{Role: historyUserRole, Kind: kind, Content: "provided context"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("restored = %#v, want %#v", got, want)
			}
		})
	}
}

func TestRestoreConversationOrdersAllTurnsWithoutSystemContext(t *testing.T) {
	t.Parallel()
	view := history.ConversationView{
		Messages: []history.Message{
			{ID: "u2", Sequence: 3, Role: historyUserRole, Kind: historyRefinementKind, Content: historyIncludeHidden},
			{ID: "system", Sequence: 0, Role: historySystemRole, Kind: "system_prompt", Content: "old configuration"},
			{ID: "a", Sequence: 2, Role: historyAssistantRole, Kind: historyResponseKind, Content: "not authoritative"},
			{ID: "u", Sequence: 1, Role: historyUserRole, Kind: historyInitialPromptKind, Content: historyListFiles},
			{ID: "u3", Sequence: 4, Role: historyUserRole, Kind: historyRefinementKind, Content: historySortBySize},
		},
		Responses: []history.Response{{MessageID: "a", PayloadJSON: `{"is_valid":true,"commands":[{"command":"ls -a","short_explanation":"list","is_dangerous":false}]}`}},
	}
	got, err := history.RestoreConversation(view)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Message{
		{Role: historyUserRole, Content: historyListFiles},
		{Role: historyAssistantRole, Kind: historyResponseKind, Content: `{"commands":["ls -a"]}`},
		{Role: historyUserRole, Content: historyIncludeHidden},
		{Role: historyUserRole, Content: historySortBySize},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("restored messages = %#v, want %#v", got, want)
	}
}
