package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/spf13/cobra"
)

func TestQueryRunRecoveryUsesCapturedContextAndDisplaysInertSuggestions(t *testing.T) {
	t.Parallel()
	const providerResponse = `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"commands\":[{\"command\":\"printf fixed\",\"short_explanation\":\"use the corrected option\",\"is_dangerous\":false}],\"is_valid\":true}"}}]}`
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request = %s %s, want POST /v1/chat/completions", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read provider request: %v", err)
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(providerResponse)); err != nil {
			t.Errorf("write provider response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	cfg := &config.Config{
		LLMProvider:    openAIKind,
		OpenAIAPIKey:   "unit-test-key",
		OpenAIModel:    "unit-test-model",
		OpenAIBaseURL:  server.URL + "/v1",
		ContextEnabled: false,
	}
	messages := []domain.Message{{Role: queryUserRole, Kind: "command_error", Content: `command="tool --bad" stderr="unknown option"`}}
	for _, action := range []string{runFailureAnalyze, runFailureFix} {
		var output strings.Builder
		cmd := &cobra.Command{}
		cmd.SetErr(&output)
		if err := queryRunRecovery(t.Context(), cmd, cfg, messages, action); err != nil {
			t.Fatalf("queryRunRecovery(%s): %v", action, err)
		}
		if !strings.Contains(output.String(), "printf fixed") || !strings.Contains(output.String(), "not executed") {
			t.Errorf("%s output did not show inert suggestions: %q", action, output.String())
		}
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(<-requests, &request); err != nil {
			t.Fatalf("decode %s request: %v", action, err)
		}
		var hasContext bool
		for _, message := range request.Messages {
			hasContext = hasContext || (strings.Contains(message.Content, "tool --bad") && strings.Contains(message.Content, "unknown option"))
		}
		if !hasContext {
			t.Errorf("%s request omitted captured error context", action)
		}
		wantTitle := "Command failure analysis"
		if action == runFailureFix {
			wantTitle = "Suggested correction"
		}
		if !strings.Contains(output.String(), wantTitle) {
			t.Errorf("%s output missing title %q: %q", action, wantTitle, output.String())
		}
	}
}
