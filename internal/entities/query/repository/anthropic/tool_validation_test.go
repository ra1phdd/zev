package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
)

const (
	toolUseBlock = `{"type":"tool_use","id":"call-1","name":"exec","input":{"command":"pwd","args":[],"reason":"context"}}`
	textBlock    = `{"type":"text","text":"{\"approved\":true}"}`
)

func TestCompleteToolTurnReviewer(t *testing.T) {
	t.Parallel()
	requests := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode review: %v", err)
			return
		}
		requests <- body
		if _, err := io.WriteString(w, messageResponse("end_turn", textBlock)); err != nil {
			t.Errorf("write review: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := New("test-key", "test-model", server.URL+"/v1", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{History: &domain.ConversationHistory{
		SystemPrompt: "isolated-review-system", Messages: []domain.Message{{Role: roleUser, Content: "review-this-command"}},
	}})
	if err != nil || turn == nil || turn.Content != `{"approved":true}` || len(turn.Calls) != 0 {
		t.Fatalf("review = (%+v, %v), want arbitrary safety JSON", turn, err)
	}
	body := <-requests
	for _, key := range []string{"tools", "tool_choice", "output_config", "output_format"} {
		if _, exists := body[key]; exists {
			t.Errorf("review unexpectedly advertises %s", key)
		}
	}
	if string(body["system"]) != `"isolated-review-system"` || strings.Contains(string(body["messages"]), "original-system") {
		t.Fatalf("review history = %s / %s", body["system"], body["messages"])
	}
}

func TestCompleteToolTurnRejectsInvalidResponses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		status    int
		body      string
		enabled   bool
		tooLarge  bool
		shortBody bool
	}{
		{name: "max tokens with complete call", body: messageResponse("max_tokens", toolUseBlock), enabled: true},
		{name: "pause turn", body: messageResponse("pause_turn", toolUseBlock), enabled: true},
		{name: "stop sequence", body: messageResponse("stop_sequence", toolUseBlock), enabled: true},
		{name: "missing stop reason", body: messageResponse("", toolUseBlock), enabled: true},
		{name: "refusal", body: messageResponse("refusal", textBlock), enabled: true},
		{name: "tool use without calls", body: messageResponse("tool_use", textBlock), enabled: true},
		{name: "end turn with calls", body: messageResponse("end_turn", toolUseBlock), enabled: true},
		{name: "disabled tool call", body: messageResponse("tool_use", toolUseBlock)},
		{name: "empty", body: messageResponse("end_turn", ""), enabled: true},
		{name: "HTTP body truncated after valid call", body: messageResponse("tool_use", toolUseBlock), enabled: true, shortBody: true},
		{name: "malformed", body: `{"secret-sensitive-payload`, enabled: true},
		{name: "truncated JSON", body: strings.TrimSuffix(messageResponse("tool_use", toolUseBlock), "}"), enabled: true},
		{name: "trailing JSON", body: messageResponse("tool_use", toolUseBlock) + `{}`, enabled: true},
		{name: "array arguments", body: messageResponse("tool_use", strings.ReplaceAll(toolUseBlock, `{"command":"pwd","args":[],"reason":"context"}`, `[]`)), enabled: true},
		{name: "missing ID", body: messageResponse("tool_use", strings.ReplaceAll(toolUseBlock, `"id":"call-1",`, "")), enabled: true},
		{name: "missing name", body: messageResponse("tool_use", strings.ReplaceAll(toolUseBlock, `"name":"exec",`, "")), enabled: true},
		{name: "duplicate IDs", body: messageResponse("tool_use", toolUseBlock+","+toolUseBlock), enabled: true},
		{name: "missing text before call", body: messageResponse("tool_use", `{"type":"text"},`+toolUseBlock), enabled: true},
		{name: "error alongside call", body: strings.TrimSuffix(messageResponse("tool_use", toolUseBlock), "}") + `,"error":{"message":"secret-sensitive-payload"}}`, enabled: true},
		{name: "valid call then malformed call", body: messageResponse("tool_use", toolUseBlock+`,{"type":"tool_use","id":"call-2","name":"exec","input":null}`), enabled: true},
		{name: "unsupported block", body: messageResponse("tool_use", toolUseBlock+`,{"type":"server_tool_use"}`), enabled: true},
		{name: "error envelope", body: `{"type":"error","error":{"message":"secret-sensitive-payload"}}`, enabled: true},
		{name: "HTTP error", status: http.StatusUnauthorized, body: `{"error":{"message":"secret-sensitive-payload"}}`, enabled: true},
		{name: "oversized envelope", body: strings.TrimSuffix(messageResponse("tool_use", toolUseBlock), "}") + `,"padding":"` + strings.Repeat("x", repository.MaxProviderResponseBytes) + `"}`, enabled: true, tooLarge: true},
		{name: "oversized trailing whitespace", body: messageResponse("tool_use", toolUseBlock) + strings.Repeat(" ", repository.MaxProviderResponseBytes), enabled: true, tooLarge: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.shortBody {
					w.Header().Set("Content-Length", strconv.Itoa(len(test.body)+10))
				}
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				if _, err := io.WriteString(w, test.body); err != nil {
					t.Errorf("write fixture: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			provider, err := New("test-key", "test-model", server.URL+"/v1", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{History: &domain.ConversationHistory{
				Messages: []domain.Message{{Role: roleUser, Content: "test"}},
			}, EnableTools: test.enabled})
			if err == nil || turn != nil {
				t.Fatalf("invalid response accepted: (%+v, %v)", turn, err)
			}
			if strings.Contains(err.Error(), "secret-sensitive-payload") || strings.Contains(err.Error(), server.URL) || len(err.Error()) > 512 {
				t.Fatalf("unsafe error: %s", err)
			}
			if test.shortBody && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("truncated body error identity lost: %v", err)
			}
			if test.tooLarge && !errors.Is(err, repository.ErrProviderResponseTooLarge) {
				t.Fatalf("size error identity lost: %v", err)
			}
		})
	}
}

func TestCompleteToolTurnCancellation(t *testing.T) {
	t.Parallel()
	for _, duringBody := range []bool{false, true} {
		name := "headers"
		if duringBody {
			name = "body"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			started := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if duringBody {
					if _, err := io.WriteString(w, `{"type":"message",`); err != nil {
						t.Errorf("write partial body: %v", err)
					}
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Errorf("flush: %v", err)
					}
				}
				close(started)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			t.Cleanup(server.Close)
			defer close(release)
			provider, err := New("test-key", "test-model", server.URL+"/v1", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := provider.CompleteToolTurn(ctx, domain.ToolRequest{History: &domain.ConversationHistory{
					Messages: []domain.Message{{Role: roleUser, Content: "test"}},
				}, EnableTools: true})
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not reach native endpoint")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation identity = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("provider did not cancel native request")
			}
		})
	}
}

func messageResponse(stop, blocks string) string {
	return `{"type":"message","role":"assistant","stop_reason":"` + stop + `","content":[` + blocks + `]}`
}
