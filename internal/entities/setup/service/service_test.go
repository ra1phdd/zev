package service

import (
	"maps"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

func TestMergeProviderChangesPreservesConcurrentUnrelatedFields(t *testing.T) {
	t.Parallel()

	before := config.ProviderConfig{Type: providerOpenAI, Model: "original"}
	after := before
	after.BaseURL, after.APIKey = "edited-url", "edited-key"
	after.HTTPHeaders = map[string]string{testHeaderName: "edited"}
	latest := before
	latest.Model = "concurrent-model"
	got := mergeProviderChanges(latest, before, after)
	if got.BaseURL != after.BaseURL || got.APIKey != after.APIKey || !maps.Equal(got.HTTPHeaders, after.HTTPHeaders) || got.Model != latest.Model {
		t.Fatal("expected profile edits to preserve the concurrent model change")
	}
}

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	const masked = "***"

	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "empty key",
			key:  "",
			want: "",
		},
		{
			name: "short key (less than 6 chars)",
			key:  "abc",
			want: masked,
		},
		{
			name: "normal OpenAI key",
			key:  "sk-proj-1234567890abcdefghijklmnop",
			want: masked,
		},
		{
			name: "normal Gemini key",
			key:  "AIzaSyAbCdEfGhIjKlMnOpQrStUvWxYz",
			want: masked,
		},
		{
			name: "exactly 6 chars",
			key:  "abcdef",
			want: masked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := maskAPIKey(tt.key)
			if got != tt.want {
				t.Errorf("maskAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
