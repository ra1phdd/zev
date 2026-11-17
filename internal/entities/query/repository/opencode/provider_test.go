package opencode_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository/opencode"
)

func TestGetOptionsDoesNotAdvertiseReadTool(t *testing.T) {
	t.Parallel()
	for _, withHistory := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "history"}[withHistory], func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if _, exists := request["tools"]; exists {
					t.Error("GetOptions advertised the removed read tool")
				}
				content := `{"commands":[{"command":"pwd","short_explanation":"show directory","is_dangerous":false}],"is_valid":true}`
				chunk, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": content}}}})
				if err != nil {
					t.Errorf("marshal chunk: %v", err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if _, err := io.WriteString(w, "data: "+string(chunk)+"\n\ndata: [DONE]\n\n"); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			provider, err := opencode.New("key", "model", server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			var response *domain.OptionsResponse
			if withHistory {
				response, err = provider.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{SystemPrompt: "environment", Messages: []domain.Message{{Role: "user", Content: "show directory"}}})
			} else {
				response, err = provider.GetOptions(t.Context(), "show directory", "environment")
			}
			if err != nil || response == nil || len(response.Commands) != 1 || response.Commands[0].Command != "pwd" {
				t.Fatalf("GetOptions() = (%+v, %v)", response, err)
			}
		})
	}
}
