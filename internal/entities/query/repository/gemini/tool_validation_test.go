package gemini

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
	callPart = `{"functionCall":{"id":"call-1","name":"exec","args":{"command":"pwd","args":[],"reason":"context"}},"thoughtSignature":"opaque-signature"}`
	textPart = `{"text":"{\"approved\":true}"}`
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
		if _, err := io.WriteString(w, contentResponse("STOP", textPart)); err != nil {
			t.Errorf("write review: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := New("test-key", "test-model", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{History: &domain.ConversationHistory{
		SystemPrompt: "isolated-review-system", Messages: []domain.Message{{Role: geminiRoleUser, Content: "review-this-command"}},
	}})
	if err != nil || turn == nil || turn.Content != `{"approved":true}` || len(turn.Calls) != 0 {
		t.Fatalf("review = (%+v, %v), want arbitrary safety JSON", turn, err)
	}
	body := <-requests
	for _, key := range []string{"tools", "toolConfig"} {
		if _, exists := body[key]; exists {
			t.Errorf("review unexpectedly advertises %s", key)
		}
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(body["generationConfig"], &config); err != nil {
		t.Fatal(err)
	}
	if string(config["response_mime_type"]) != `"application/json"` || len(config) != 1 {
		t.Fatalf("review must request plain JSON, not options schema: %s", body["generationConfig"])
	}
	if !strings.Contains(string(body[geminiJSONSystem]), "isolated-review-system") || strings.Contains(string(body[geminiJSONContents]), "original-system") {
		t.Fatalf("review history = %s / %s", body[geminiJSONSystem], body[geminiJSONContents])
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
		{name: "max tokens with complete call", body: contentResponse("MAX_TOKENS", callPart), enabled: true},
		{name: "missing finish reason", body: contentResponse("", callPart), enabled: true},
		{name: "safety with complete call", body: contentResponse("SAFETY", callPart), enabled: true},
		{name: "malformed function call finish", body: contentResponse("MALFORMED_FUNCTION_CALL", callPart), enabled: true},
		{name: "unknown finish", body: contentResponse("secret-sensitive-payload", callPart), enabled: true},
		{name: "disabled tool call", body: contentResponse("STOP", callPart)},
		{name: "empty parts", body: contentResponse("STOP", ""), enabled: true},
		{name: "no candidates", body: `{"candidates":[]}`, enabled: true},
		{name: "missing role", body: strings.ReplaceAll(contentResponse("STOP", callPart), `"role":"model",`, ""), enabled: true},
		{name: "HTTP body truncated after valid call", body: contentResponse("STOP", callPart), enabled: true, shortBody: true},
		{name: "malformed", body: `{"secret-sensitive-payload`, enabled: true},
		{name: "truncated JSON", body: strings.TrimSuffix(contentResponse("STOP", callPart), "}"), enabled: true},
		{name: "trailing JSON", body: contentResponse("STOP", callPart) + `{}`, enabled: true},
		{name: "null arguments", body: contentResponse("STOP", strings.ReplaceAll(callPart, `{"command":"pwd","args":[],"reason":"context"}`, `null`)), enabled: true},
		{name: "array arguments", body: contentResponse("STOP", strings.ReplaceAll(callPart, `{"command":"pwd","args":[],"reason":"context"}`, `[]`)), enabled: true},
		{name: "missing name", body: contentResponse("STOP", strings.ReplaceAll(callPart, `"name":"exec",`, "")), enabled: true},
		{name: "duplicate IDs", body: contentResponse("STOP", callPart+","+callPart), enabled: true},
		{name: "prompt blocked", body: strings.TrimSuffix(contentResponse("STOP", callPart), "}") + `,"promptFeedback":{"blockReason":"SAFETY"}}`, enabled: true},
		{name: "error envelope", body: `{"error":{"message":"secret-sensitive-payload"}}`, enabled: true},
		{name: "HTTP error", status: http.StatusUnauthorized, body: `{"error":{"message":"secret-sensitive-payload"}}`, enabled: true},
		{name: "oversized envelope", body: strings.TrimSuffix(contentResponse("STOP", callPart), "}") + `,"padding":"` + strings.Repeat("x", repository.MaxProviderResponseBytes) + `"}`, enabled: true, tooLarge: true},
		{name: "oversized trailing whitespace", body: contentResponse("STOP", callPart) + strings.Repeat(" ", repository.MaxProviderResponseBytes), enabled: true, tooLarge: true},
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
			provider, err := New("test-key", "test-model", server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{History: &domain.ConversationHistory{
				Messages: []domain.Message{{Role: geminiRoleUser, Content: "test"}},
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
					if _, err := io.WriteString(w, `{"candidates":[`); err != nil {
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
			provider, err := New("test-key", "test-model", server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := provider.CompleteToolTurn(ctx, domain.ToolRequest{History: &domain.ConversationHistory{
					Messages: []domain.Message{{Role: geminiRoleUser, Content: "test"}},
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

func contentResponse(finish, parts string) string {
	return `{"candidates":[{"finishReason":"` + finish + `","content":{"role":"model","parts":[` + parts + `]}}]}`
}
