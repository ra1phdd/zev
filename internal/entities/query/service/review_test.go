package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

type reviewToolProvider struct {
	request domain.ToolRequest
	turn    *domain.ToolTurn
	err     error
}

func (p *reviewToolProvider) CompleteToolTurn(_ context.Context, request domain.ToolRequest) (*domain.ToolTurn, error) {
	p.request = request
	return p.turn, p.err
}

func (p *reviewToolProvider) GetOptions(context.Context, string, string) (*domain.OptionsResponse, error) {
	return nil, errors.New("unexpected options request")
}

func (p *reviewToolProvider) GetOptionsWithHistory(context.Context, *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	return nil, errors.New("unexpected options request")
}

func (p *reviewToolProvider) Name() string { return "test" }

func (p *reviewToolProvider) Model() string { return "test" }

func TestReviewCommandSafety(t *testing.T) {
	t.Parallel()
	provider := &reviewToolProvider{turn: &domain.ToolTurn{Content: `{"safe":true,"reason":"The command only lists matching files."}`}}
	got, err := ReviewCommandSafety(t.Context(), provider, `find . -name '*.go'`, "Find Go files", "List matching Go source files")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Safe {
		t.Fatalf("expected safe review, got %#v", got)
	}
	if provider.request.EnableTools || provider.request.EnableWebFetch {
		t.Fatal("safety review must not enable tools or web fetch")
	}
	if provider.request.History == nil || provider.request.History.SystemPrompt == "" || len(provider.request.History.Messages) != 1 {
		t.Fatalf("unexpected review history: %#v", provider.request.History)
	}
	content := provider.request.History.Messages[0].Content
	for _, want := range []string{`find . -name '*.go'`, "Find Go files", "List matching Go source files"} {
		if !strings.Contains(content, want) {
			t.Errorf("review request missing %q", want)
		}
	}
}

func TestReviewCommandSafetyFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		turn *domain.ToolTurn
		err  error
	}{
		{name: "unsafe", turn: &domain.ToolTurn{Content: `{"safe":false,"reason":"It deletes files."}`}},
		{name: "malformed", turn: &domain.ToolTurn{Content: `not json`}},
		{name: "duplicate field", turn: &domain.ToolTurn{Content: `{"safe":false,"safe":true,"reason":"ambiguous"}`}},
		{name: "unknown field", turn: &domain.ToolTurn{Content: `{"safe":true,"reason":"ok","extra":true}`}},
		{name: "missing reason", turn: &domain.ToolTurn{Content: `{"safe":true}`}},
		{name: "tool call", turn: &domain.ToolTurn{Content: `{"safe":true,"reason":"ok"}`, Calls: []domain.ToolCall{{Name: "exec"}}}},
		{name: "upstream failure", err: errors.New("provider unavailable")},
		{name: "empty response", turn: &domain.ToolTurn{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &reviewToolProvider{turn: tt.turn, err: tt.err}
			got, err := ReviewCommandSafety(t.Context(), provider, "unknown-command", "task", "description")
			if err == nil && got.Safe {
				t.Fatal("review must not authorize this response")
			}
		})
	}
}
