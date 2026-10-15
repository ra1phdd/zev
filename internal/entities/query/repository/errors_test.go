package repository_test

import (
	"context"
	"errors"
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
)

func TestWrapNetworkError(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		err          error
		wantContains string
	}{
		{
			name:         "context timeout",
			providerName: "OpenAI",
			err:          context.DeadlineExceeded,
			wantContains: "OpenAI did not respond within timeout",
		},
		{
			name:         "context cancelled",
			providerName: "Gemini",
			err:          context.Canceled,
			wantContains: "request to Gemini was cancelled",
		},
		{
			name:         "DNS error",
			providerName: "OpenCode",
			err:          &net.DNSError{Err: "no such host", Name: "api.opencode.ai"},
			wantContains: "cannot resolve OpenCode hostname",
		},
		{
			name:         "connection refused",
			providerName: "Ollama",
			err:          &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
			wantContains: "connection to Ollama refused",
		},
		{
			name:         "connection timeout",
			providerName: "OpenAI",
			err:          &net.OpError{Op: "dial", Err: syscall.ETIMEDOUT},
			wantContains: "OpenAI connection timed out",
		},
		{
			name:         "URL timeout",
			providerName: "Gemini",
			err:          &url.Error{Op: "Post", URL: "https://api.gemini.com", Err: context.DeadlineExceeded},
			wantContains: "Gemini did not respond within timeout", // DeadlineExceeded проверяется раньше
		},
		{
			name:         "generic error",
			providerName: "OpenAI",
			err:          errors.New("unknown error"),
			wantContains: "OpenAI API error: unknown error",
		},
		{
			name:         "nil error",
			providerName: "OpenAI",
			err:          nil,
			wantContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repository.WrapNetworkError(tt.providerName, tt.err)

			if tt.err == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}

			if got == nil {
				t.Errorf("expected error, got nil")
				return
			}

			if tt.wantContains != "" && !contains(got.Error(), tt.wantContains) {
				t.Errorf("error = %q, want to contain %q", got.Error(), tt.wantContains)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
