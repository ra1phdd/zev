package factory

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/httpclient"
)

func TestProvidersRejectInvalidResponsesBeforeReturningToService(t *testing.T) {
	t.Parallel()

	providers := providerTestCases()

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

	invalidPayload, err := json.Marshal(domain.OptionsResponse{
		Commands: []domain.Command{{Command: "", ShortExplanation: "missing command"}},
		IsValid:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidJSON := string(invalidPayload)
	for _, providerCase := range providers {
		for _, methodCase := range methods {
			t.Run(providerCase.name+"/"+methodCase.name, func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if _, err := io.WriteString(w, providerCase.wire(invalidJSON)); err != nil {
						t.Errorf("writing response: %v", err)
					}
				}))
				defer server.Close()

				cfg := &config.Config{}
				providerCase.setup(cfg, server.URL)
				provider, err := NewProvider(cfg, httpclient.New(nil))
				if err != nil {
					t.Fatalf("NewProvider() error = %v", err)
				}

				response, err := methodCase.call(t.Context(), provider)
				if err == nil {
					t.Fatal("provider returned nil error for invalid response")
				}
				if response != nil {
					t.Fatalf("provider returned response %#v for invalid response", response)
				}
				if !errors.Is(err, domain.ErrInvalidOptionsResponse) {
					t.Fatalf("error = %v, want ErrInvalidOptionsResponse", err)
				}
				if strings.Contains(err.Error(), "missing command") {
					t.Fatalf("provider error leaked response contents: %v", err)
				}
			})
		}
	}
}
