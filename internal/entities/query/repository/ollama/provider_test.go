package ollama_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/query/repository/ollama"
)

func TestNewConstructorGuards(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		baseURL string
		model   string
		client  *http.Client
		wantErr string
	}{
		{name: "missing URL", model: "local-model", client: &http.Client{}, wantErr: "OLLAMA_BASE_URL"},
		{name: "missing model", baseURL: "http://localhost:11434/v1", client: &http.Client{}, wantErr: "OLLAMA_MODEL"},
		{name: "missing client", baseURL: "http://localhost:11434/v1", model: "local-model", wantErr: "httpClient"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := ollama.New(tt.baseURL, tt.model, tt.client)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("New() error = %v, want %q", err, tt.wantErr)
			}
			if provider != nil {
				t.Fatalf("New() provider = %#v, want nil", provider)
			}
		})
	}
}
