package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/httpclient"
)

func TestProviderAutomaticHTTPRetry(t *testing.T) {
	t.Parallel()
	providers := []struct {
		name  string
		setup func(*config.Config, string)
		wire  func(string) string
	}{
		{
			name: "openai",
			setup: func(cfg *config.Config, baseURL string) {
				cfg.LLMProvider = "openai"
				cfg.OpenAIAPIKey = "test-openai-key"
				cfg.OpenAIModel = "test-openai-model"
				cfg.OpenAIBaseURL = baseURL
			},
			wire: openAIResponse,
		},
		{
			name: "gemini",
			setup: func(cfg *config.Config, baseURL string) {
				cfg.LLMProvider = "gemini"
				cfg.GeminiAPIKey = "test-gemini-key"
				cfg.GeminiModel = "test-gemini-model"
				cfg.GeminiBaseURL = baseURL
			},
			wire: geminiResponse,
		},
		{
			name: "anthropic",
			setup: func(cfg *config.Config, baseURL string) {
				cfg.LLMProvider = "anthropic"
				cfg.AnthropicAPIKey = "test-anthropic-key"
				cfg.AnthropicModel = "test-anthropic-model"
				cfg.AnthropicBaseURL = baseURL
			},
			wire: anthropicResponse,
		},
		{
			name: "ollama",
			setup: func(cfg *config.Config, baseURL string) {
				cfg.LLMProvider = "ollama"
				cfg.OllamaModel = "test-ollama-model"
				cfg.OllamaBaseURL = baseURL
			},
			wire: openAIResponse,
		},
		{
			name: "opencode",
			setup: func(cfg *config.Config, baseURL string) {
				cfg.LLMProvider = "opencode"
				cfg.OpenAIAPIKey = "test-opencode-key"
				cfg.OpenAIModel = "test-opencode-model"
				cfg.OpenAIBaseURL = baseURL
			},
			wire: openCodeResponse,
		},
	}

	methods := []struct {
		name string
		call func(context.Context, domain.InferenceProvider) (*domain.OptionsResponse, error)
	}{
		{
			name: "GetOptions",
			call: func(ctx context.Context, provider domain.InferenceProvider) (*domain.OptionsResponse, error) {
				return provider.GetOptions(ctx, "list files", "test environment")
			},
		},
		{
			name: "GetOptionsWithHistory",
			call: func(ctx context.Context, provider domain.InferenceProvider) (*domain.OptionsResponse, error) {
				return provider.GetOptionsWithHistory(ctx, &domain.ConversationHistory{
					SystemPrompt: "You are a shell assistant.",
					Messages: []domain.Message{
						{Role: "user", Content: "list files"},
					},
				})
			},
		},
	}

	for _, providerCase := range providers {
		for _, methodCase := range methods {
			t.Run(providerCase.name+"/"+methodCase.name, func(t *testing.T) {
				t.Parallel()

				var (
					mu             sync.Mutex
					requestBodies  [][]byte
					requestHeaders []string
				)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, "failed to read request", http.StatusInternalServerError)
						return
					}

					mu.Lock()
					requestBodies = append(requestBodies, body)
					requestHeaders = append(requestHeaders, r.Header.Get("X-Retry-Test"))
					attempt := len(requestBodies)
					mu.Unlock()

					switch attempt {
					case 1:
						http.Error(w, "rate limited", http.StatusTooManyRequests)
					case 2:
						http.Error(w, "temporary failure", http.StatusInternalServerError)
					default:
						w.Header().Set("Content-Type", "application/json")
						if _, err := io.WriteString(w, providerCase.wire(testOptionsJSON())); err != nil {
							t.Errorf("writing response: %v", err)
						}
					}
				}))
				defer server.Close()

				cfg := &config.Config{
					HTTPHeaders: map[string]string{
						"X-Retry-Test": "preserved-header",
					},
				}
				providerCase.setup(cfg, server.URL)

				provider, err := NewProvider(cfg, httpclient.New(cfg.HTTPHeaders))
				if err != nil {
					t.Fatalf("NewProvider() error = %v", err)
				}

				response, err := methodCase.call(t.Context(), provider)
				if err != nil {
					t.Fatalf("%s() error after retries: %v", methodCase.name, err)
				}
				if response == nil || !response.IsValid {
					t.Fatalf("%s() response = %#v, want valid response", methodCase.name, response)
				}
				if len(response.Commands) != 1 || response.Commands[0].Command != "ls -la" {
					t.Fatalf("%s() response commands = %#v, want one ls -la command", methodCase.name, response.Commands)
				}

				mu.Lock()
				bodies := append([][]byte(nil), requestBodies...)
				headers := append([]string(nil), requestHeaders...)
				mu.Unlock()

				if len(bodies) != 3 {
					t.Fatalf("request attempts = %d, want 3", len(bodies))
				}
				if len(headers) != 3 {
					t.Fatalf("captured headers = %d, want 3", len(headers))
				}
				for attempt := range bodies {
					if !bytes.Equal(bodies[0], bodies[attempt]) {
						t.Errorf("request body attempt %d differs from first attempt", attempt+1)
					}
					if headers[attempt] != "preserved-header" {
						t.Errorf("X-Retry-Test attempt %d = %q, want %q", attempt+1, headers[attempt], "preserved-header")
					}
				}
			})
		}
	}
}

func testOptionsJSON() string {
	payload, err := json.Marshal(domain.OptionsResponse{
		Commands: []domain.Command{{
			Command:          "ls -la",
			ShortExplanation: "List files",
			IsDangerous:      false,
		}},
		IsValid: true,
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

func openAIResponse(options string) string {
	return `{"choices":[{"message":{"content":` + strconvQuote(options) + `}}]}`
}

func anthropicResponse(options string) string {
	return `{"content":[{"type":"text","text":` + strconvQuote(options) + `}]}`
}

func geminiResponse(options string) string {
	return `{"candidates":[{"content":{"parts":[{"text":` + strconvQuote(options) + `}]}}]}`
}

func openCodeResponse(options string) string {
	return "data: {\"choices\":[{\"delta\":{\"content\":" + strconvQuote(options) + "}}]}\n\ndata: [DONE]\n\n"
}

func strconvQuote(value string) string {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(payload)
}
