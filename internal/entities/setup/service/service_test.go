package service

import (
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

func TestMergeProviderChangesPreservesConcurrentUnrelatedFields(t *testing.T) {
	t.Parallel()

	original := config.DefaultConfig()
	changed := *original
	changed.OpenAIModel = "edited-model"
	changed.OpenAIAPIKey = "edited-key"
	changed.GeminiModel = "should-not-be-written"
	latest := *original
	latest.OpenAIBaseURL = "concurrent-base-url"
	latest.CommandExecutionMode = "execute"
	latest.GeminiModel = "concurrent-gemini-model"

	mergeProviderChanges(&latest, original, &changed, "openai")

	if latest.OpenAIModel != changed.OpenAIModel || latest.OpenAIAPIKey != changed.OpenAIAPIKey {
		t.Fatal("selected provider changes were not merged")
	}
	if latest.OpenAIBaseURL != "concurrent-base-url" {
		t.Fatal("concurrent change to an unchanged field was overwritten")
	}
	if latest.GeminiModel != "concurrent-gemini-model" || latest.CommandExecutionMode != "execute" {
		t.Fatal("unrelated concurrent field was overwritten")
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
