package factory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
)

func TestOllamaQueryWiring(t *testing.T) {
	t.Parallel()

	const (
		providerName = "ollama"
		userRole     = "user"
		model        = "ollama-query-model"
		command      = "ls -la"
		prompt       = "show hidden files"
		envContext   = "test-os; shell=test-shell; cwd=/query-fixture"
	)
	history := &domain.ConversationHistory{
		SystemPrompt: "Preserve this query system prompt.",
		Messages: []domain.Message{
			{Role: userRole, Content: "show visible files"},
			{Role: "assistant", Content: `{"commands":["ls"]}`},
			{Role: userRole, Content: prompt},
		},
	}

	for _, tt := range []struct {
		name         string
		call         func(context.Context, domain.InferenceProvider) (*domain.OptionsResponse, error)
		wantMessages []domain.Message
	}{
		{
			name: "initial query",
			call: func(ctx context.Context, provider domain.InferenceProvider) (*domain.OptionsResponse, error) {
				return provider.GetOptions(ctx, prompt, envContext)
			},
			wantMessages: []domain.Message{
				{Role: "system", Content: fmt.Sprintf(repository.SystemPrompt, envContext)},
				{Role: userRole, Content: prompt},
			},
		},
		{
			name: "continued query",
			call: func(ctx context.Context, provider domain.InferenceProvider) (*domain.OptionsResponse, error) {
				return provider.GetOptionsWithHistory(ctx, history)
			},
			wantMessages: append([]domain.Message{{Role: "system", Content: history.SystemPrompt}}, history.Messages...),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			type queryRequest struct {
				Model    string           `json:"model"`
				Messages []domain.Message `json:"messages"`
			}
			requests := make(chan queryRequest, 2)
			var requestCount atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/ollama/v1/chat/completions" {
					t.Errorf("endpoint = %s %s, want POST /ollama/v1/chat/completions", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer ollama" {
					t.Errorf("Authorization = %q, want Ollama placeholder, not OpenAI key", got)
				}
				var request queryRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decoding query request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- request
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, openAIResponse(testOptionsJSON())); err != nil {
					t.Errorf("writing query response: %v", err)
				}
			}))
			t.Cleanup(server.Close)

			cfg := &config.Config{
				LLMProvider:   providerName,
				OllamaModel:   model,
				OllamaBaseURL: server.URL + "/ollama/v1",
				OpenAIAPIKey:  "unrelated-openai-key",
				OpenAIModel:   "unrelated-openai-model",
				OpenAIBaseURL: server.URL + "/openai/v1",
			}
			provider, err := NewProvider(cfg, server.Client())
			if err != nil {
				t.Fatalf("NewProvider() error = %v", err)
			}
			if got := provider.Model(); got != model {
				t.Errorf("Model() = %q, want %q", got, model)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			response, err := tt.call(ctx, provider)
			if err != nil {
				t.Fatalf("query error = %v", err)
			}
			want := &domain.OptionsResponse{
				IsValid:  true,
				Commands: []domain.Command{{Command: command, ShortExplanation: "List files"}},
			}
			if !reflect.DeepEqual(response, want) {
				t.Errorf("response = %#v, want %#v", response, want)
			}
			select {
			case request := <-requests:
				if request.Model != model || !reflect.DeepEqual(request.Messages, tt.wantMessages) {
					t.Errorf("request = %#v, want model %q and messages %#v", request, model, tt.wantMessages)
				}
			default:
				t.Fatal("query did not reach configured Ollama endpoint")
			}

			cancel()
			response, err = tt.call(ctx, provider)
			if response != nil || !errors.Is(err, context.Canceled) {
				t.Errorf("cancelled query = (%#v, %v), want nil response and context.Canceled", response, err)
			}
			if got := requestCount.Load(); got != 1 {
				t.Errorf("HTTP requests = %d, want one successful query and no cancelled request", got)
			}
		})
	}
}
