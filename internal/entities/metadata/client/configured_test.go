package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/entities/metadata/service"
)

const (
	testOpenAIModel    = "openai-model"
	testOllamaProvider = "ollama"
)

func TestConfiguredProviderProtocols(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		provider  string
		path      string
		model     string
		bearer    string
		gemini    string
		anthropic string
	}{
		{provider: "openai", path: "/openai/v1/chat/completions", model: testOpenAIModel, bearer: "Bearer openai-key"},
		{provider: "opencode", path: "/openai/v1/chat/completions", model: testOpenAIModel, bearer: "Bearer openai-key"},
		{provider: testOllamaProvider, path: "/ollama/v1/chat/completions", model: "ollama-model"},
		{provider: providerGemini, path: "/gemini/v1beta/models/gemini-model:generateContent", model: "gemini-model", gemini: "gemini-key"},
		{provider: providerAnthropic, path: "/anthropic/v1/messages", model: "anthropic-model", anthropic: "anthropic-key"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("request routing = %s %s, content type %q", r.Method, r.URL, r.Header.Get("Content-Type"))
				}
				if r.Header.Get("Authorization") != tc.bearer || r.Header.Get("x-goog-api-key") != tc.gemini || r.Header.Get("x-api-key") != tc.anthropic {
					t.Error("provider authentication headers do not match selected provider")
				}
				wantVersion := ""
				if tc.provider == providerAnthropic {
					wantVersion = "2023-06-01"
				}
				if got := r.Header.Get("anthropic-version"); got != wantVersion {
					t.Errorf("anthropic-version = %q, want %q", got, wantVersion)
				}
				var fields map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "decode request", http.StatusBadRequest)
					return
				}
				for _, field := range []string{"tools", "history", "environment", "api_key"} {
					if _, exists := fields[field]; exists {
						t.Errorf("unexpected request field %q", field)
					}
				}
				if tc.provider != providerGemini {
					var model string
					if err := json.Unmarshal(fields["model"], &model); err != nil || model != tc.model {
						t.Errorf("model = %q, %v; want %q", model, err, tc.model)
					}
				}
				switch tc.provider {
				case providerGemini:
					writeString(t, w, `{"candidates":[{"content":{"parts":[{"text":"{\"description\":\"generated\"}"}]}}]}`)
				case providerAnthropic:
					writeString(t, w, `{"content":[{"type":"text","text":"{\"description\":\"generated\"}"}]}`)
				default:
					writeString(t, w, `{"choices":[{"message":{"content":"{\"description\":\"generated\"}"}}]}`)
				}
			}))
			t.Cleanup(server.Close)
			cfg := config.DefaultConfig()
			cfg.LLMProvider = tc.provider
			cfg.OpenAIBaseURL, cfg.OllamaBaseURL = server.URL+"/openai/v1/", server.URL+"/ollama/v1/"
			cfg.GeminiBaseURL, cfg.AnthropicBaseURL = server.URL+"/gemini/", server.URL+"/anthropic/v1/"
			cfg.OpenAIModel, cfg.OllamaModel = testOpenAIModel, "ollama-model"
			cfg.GeminiModel, cfg.AnthropicModel = "gemini-model", "anthropic-model"
			cfg.OpenAIAPIKey, cfg.GeminiAPIKey, cfg.AnthropicAPIKey = "openai-key", "gemini-key", "anthropic-key"
			generator, err := NewConfigured(cfg, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			got, err := generator.GenerateDescription(t.Context(), service.DescriptionInput{EntityKind: "alias", EntityName: "sample", Template: testTemplate})
			if err != nil || got != testGenerated {
				t.Fatalf("description=%q err=%v", got, err)
			}
		})
	}
}

func TestConfiguredRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		change    func(*config.Config)
		nilConfig bool
		nilClient bool
	}{
		{name: "nil config", nilConfig: true},
		{name: "nil client", nilClient: true},
		{name: "unknown provider", change: func(cfg *config.Config) { cfg.LLMProvider = "unknown-private-value" }},
		{name: "empty provider", change: func(cfg *config.Config) { cfg.LLMProvider = "" }},
		{name: "openai model", change: func(cfg *config.Config) { cfg.OpenAIModel = " \t" }},
		{name: "openai URL", change: func(cfg *config.Config) { cfg.OpenAIBaseURL = "https://private.example/?token=secret" }},
		{name: "opencode model", change: func(cfg *config.Config) { cfg.LLMProvider = "opencode"; cfg.OpenAIModel = "" }},
		{name: "ollama model", change: func(cfg *config.Config) { cfg.LLMProvider = testOllamaProvider; cfg.OllamaModel = "" }},
		{name: "ollama URL", change: func(cfg *config.Config) { cfg.LLMProvider = testOllamaProvider; cfg.OllamaBaseURL = "" }},
		{name: "gemini model", change: func(cfg *config.Config) { cfg.LLMProvider = providerGemini; cfg.GeminiModel = "" }},
		{name: "gemini URL", change: func(cfg *config.Config) {
			cfg.LLMProvider = providerGemini
			cfg.GeminiBaseURL = "ftp://private.example"
		}},
		{name: "anthropic model", change: func(cfg *config.Config) { cfg.LLMProvider = providerAnthropic; cfg.AnthropicModel = "" }},
		{name: "anthropic URL", change: func(cfg *config.Config) {
			cfg.LLMProvider = providerAnthropic
			cfg.AnthropicBaseURL = "https://user:secret@private.example"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.DefaultConfig()
			cfg.LLMProvider = "openai"
			cfg.OpenAIAPIKey = testPrivateAPIKey
			if tc.change != nil {
				tc.change(cfg)
			}
			if tc.nilConfig {
				cfg = nil
			}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("constructor performed HTTP I/O")
				return nil, service.ErrUnavailable
			})}
			if tc.nilClient {
				client = nil
			}
			_, err := NewConfigured(cfg, client)
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			for _, secret := range []string{testPrivateAPIKey, "private.example", testAPIKey, "unknown-private-value"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("constructor error leaked configuration: %v", err)
				}
			}
		})
	}
}
