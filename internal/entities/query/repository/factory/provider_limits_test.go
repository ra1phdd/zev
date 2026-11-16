package factory

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
)

const (
	paddingField       = "provider-envelope-padding"
	opencodeProvider   = "opencode"
	choicesField       = "choices"
	geminiProviderName = "gemini"
	geminiModelName    = "test-gemini-model"
)

func TestProvidersRejectOversizedHTTPEnvelopes(t *testing.T) {
	t.Parallel()

	for _, providerCase := range providerTestCases() {
		t.Run(providerCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := io.WriteString(w, oversizedProviderResponse(providerCase)); err != nil {
					t.Errorf("writing oversized response: %v", err)
				}
			}))
			t.Cleanup(server.Close)

			cfg := &config.Config{}
			providerCase.setup(cfg, server.URL)
			provider, err := NewProvider(cfg, httpclient.New(nil))
			if err != nil {
				t.Fatalf("NewProvider() error = %v", err)
			}

			response, err := provider.GetOptions(t.Context(), "list files", "test environment")
			if err == nil || response != nil {
				t.Fatalf("GetOptions() = (%v, %v), want nil response and error", response, err)
			}
			if len(err.Error()) > 512 {
				t.Fatalf("oversized response leaked into error: length=%d", len(err.Error()))
			}
		})
	}
}

func oversizedProviderResponse(providerCase providerTestCase) string {
	wire := providerCase.wire(testOptionsJSON())
	if providerCase.name == opencodeProvider {
		chunk := marshalProviderJSON(map[string]any{
			choicesField: []any{map[string]any{
				"delta": map[string]string{wireContent: strings.Repeat("x", repository.MaxProviderResponseBytes)},
			}},
		})
		return "data: " + chunk + "\n\ndata: [DONE]\n\n"
	}
	return strings.TrimSuffix(wire, "}") + ",\"" + paddingField + "\":\"" + strings.Repeat("x", repository.MaxProviderResponseBytes) + "\"}"
}

func TestGeminiDoesNotPutAPIKeyInRequestURL(t *testing.T) {
	t.Parallel()

	const apiKey = "gemini-secret-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("request query = %q, want no query parameters", r.URL.RawQuery)
		}
		if got := r.Header.Get("x-goog-api-key"); got != apiKey {
			t.Errorf("x-goog-api-key = %q, want configured key", got)
		}
		if _, err := io.WriteString(w, geminiResponse(testOptionsJSON())); err != nil {
			t.Errorf("writing Gemini response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	cfg := &config.Config{
		LLMProvider:   geminiProviderName,
		GeminiAPIKey:  apiKey,
		GeminiModel:   geminiModelName,
		GeminiBaseURL: server.URL,
	}
	provider, err := NewProvider(cfg, httpclient.New(nil))
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if _, err := provider.GetOptions(t.Context(), "list files", "test environment"); err != nil {
		t.Fatalf("GetOptions() error = %v", err)
	}
}

func TestGeminiRequestConstructionDoesNotLeakAPIKey(t *testing.T) {
	t.Parallel()

	const apiKey = "gemini-construction-secret"
	cfg := &config.Config{
		LLMProvider:   geminiProviderName,
		GeminiAPIKey:  apiKey,
		GeminiModel:   geminiModelName,
		GeminiBaseURL: "://malformed-base-url",
	}
	provider, err := NewProvider(cfg, httpclient.New(nil))
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	_, err = provider.GetOptions(t.Context(), "list files", "test environment")
	if err == nil {
		t.Fatal("GetOptions() error = nil, want request-construction error")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("request-construction error leaked API key: %v", err)
	}
	if strings.Contains(err.Error(), "malformed-base-url") {
		t.Fatalf("request-construction error leaked base URL: %v", err)
	}
}
