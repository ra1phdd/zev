package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const setupClientDuplicateModel = "gpt-z"

func writeSetupResponse(t *testing.T, w io.Writer, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func TestListModelsUsesProviderCatalogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   string
		basePath   string
		apiKey     string
		wantPath   string
		response   any
		wantModels []string
	}{
		{
			name:       "OpenAI",
			provider:   ProviderOpenAI,
			basePath:   "/openai/v1",
			apiKey:     "openai-secret",
			wantPath:   "/openai/v1/models",
			response:   openAIModelsResponseJSON([]string{setupClientDuplicateModel, "gpt-a", setupClientDuplicateModel}),
			wantModels: []string{"gpt-a", setupClientDuplicateModel},
		},
		{
			name:       "Gemini",
			provider:   ProviderGemini,
			basePath:   "/gemini",
			apiKey:     "gemini-secret",
			wantPath:   "/gemini/v1beta/models",
			response:   `{"models":[{"name":"models/gemini-pro","supportedGenerationMethods":["generateContent"]},{"name":"models/embed","supportedGenerationMethods":["embedContent"]}]}`,
			wantModels: []string{"gemini-pro"},
		},
		{
			name:       "Anthropic",
			provider:   ProviderAnthropic,
			basePath:   "/anthropic/v1",
			apiKey:     "anthropic-secret",
			wantPath:   "/anthropic/v1/models",
			response:   `{"data":[{"id":"claude-z"},{"id":"claude-a"}],"has_more":false}`,
			wantModels: []string{"claude-a", "claude-z"},
		},
		{
			name:       "Ollama",
			provider:   ProviderOllama,
			basePath:   "/ollama/v1",
			wantPath:   "/ollama/api/tags",
			response:   `{"models":[{"name":"llama3.2"},{"model":"qwen2.5"}]}`,
			wantModels: []string{"llama3.2", "qwen2.5"},
		},
		{
			name:       "OpenCode",
			provider:   ProviderOpenCode,
			basePath:   "/inference/openai/v1",
			apiKey:     "opencode-secret",
			wantPath:   "/inference/v1/models",
			response:   openAIModelsResponseJSON([]string{"big-pickle", "claude-sonnet"}),
			wantModels: []string{"big-pickle", "claude-sonnet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.wantPath {
					t.Errorf("request path = %q, want %q", r.URL.Path, tt.wantPath)
				}
				if got := r.Header.Get("Authorization"); tt.apiKey != "" && got != "Bearer "+tt.apiKey && tt.provider != ProviderGemini && tt.provider != ProviderAnthropic {
					t.Errorf("Authorization = %q", got)
				}
				if tt.provider == ProviderGemini && r.Header.Get("x-goog-api-key") != tt.apiKey {
					t.Errorf("Gemini API key header = %q", r.Header.Get("x-goog-api-key"))
				}
				if tt.provider == ProviderAnthropic && r.Header.Get("x-api-key") != tt.apiKey {
					t.Errorf("Anthropic API key header = %q", r.Header.Get("x-api-key"))
				}
				w.Header().Set("Content-Type", "application/json")
				response, ok := tt.response.(string)
				if !ok {
					t.Errorf("response fixture has type %T, want string", tt.response)
					return
				}
				writeSetupResponse(t, w, response)
			}))
			defer server.Close()

			models, err := New(server.Client()).ListModels(t.Context(), Request{
				Provider: tt.provider,
				APIKey:   tt.apiKey,
				BaseURL:  server.URL + tt.basePath,
			})
			if err != nil {
				t.Fatalf("ListModels() error = %v", err)
			}
			if strings.Join(models, ",") != strings.Join(tt.wantModels, ",") {
				t.Fatalf("models = %#v, want %#v", models, tt.wantModels)
			}
		})
	}
}

func TestListModelsPaginatesGeminiAndAnthropic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		basePath string
		handler  http.HandlerFunc
		want     []string
	}{
		{
			name:     "Gemini",
			provider: ProviderGemini,
			basePath: "/gemini",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("pageToken") == "next" {
					writeSetupResponse(t, w, `{"models":[{"name":"models/gemini-2","supportedGenerationMethods":["generateContent"]}]}`)
					return
				}
				writeSetupResponse(t, w, `{"models":[{"name":"models/gemini-1","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"next"}`)
			},
			want: []string{"gemini-1", "gemini-2"},
		},
		{
			name:     "Anthropic",
			provider: ProviderAnthropic,
			basePath: "/anthropic/v1",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("after_id") == "claude-1" {
					writeSetupResponse(t, w, `{"data":[{"id":"claude-2"}],"has_more":false,"last_id":"claude-2"}`)
					return
				}
				writeSetupResponse(t, w, `{"data":[{"id":"claude-1"}],"has_more":true,"last_id":"claude-1"}`)
			},
			want: []string{"claude-1", "claude-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			models, err := New(server.Client()).ListModels(t.Context(), Request{
				Provider: tt.provider,
				APIKey:   "secret",
				BaseURL:  server.URL + tt.basePath,
			})
			if err != nil {
				t.Fatalf("ListModels() error = %v", err)
			}
			if strings.Join(models, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("models = %#v, want %#v", models, tt.want)
			}
		})
	}
}

func TestListModelsRejectsOversizedAndFailedResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		body      string
		wantInErr string
	}{
		{name: "HTTP failure", status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`, wantInErr: "HTTP 401"},
		{name: "invalid JSON", status: http.StatusOK, body: "not-json", wantInErr: "decode response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				writeSetupResponse(t, w, tt.body)
			}))
			defer server.Close()
			_, err := New(server.Client()).ListModels(t.Context(), Request{
				Provider: ProviderOpenAI,
				APIKey:   "secret",
				BaseURL:  server.URL,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Fatalf("ListModels() error = %v, want substring %q", err, tt.wantInErr)
			}
		})
	}
}

func openAIModelsResponseJSON(models []string) string {
	data, err := json.Marshal(struct {
		Data []map[string]string `json:"data"`
	}{Data: func() []map[string]string {
		result := make([]map[string]string, 0, len(models))
		for _, model := range models {
			result = append(result, map[string]string{"id": model})
		}
		return result
	}()})
	if err != nil {
		panic(err)
	}
	return string(data)
}
