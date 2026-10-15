package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ra1phdd/zev-go/internal/core/domain"
)

// mockProvider для тестов.
type mockProvider struct {
	name     string
	model    string
	response *domain.OptionsResponse
	err      error
}

func (m *mockProvider) GetOptions(_ context.Context, prompt, _ string) (*domain.OptionsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Model() string {
	return m.model
}

func TestGetOptions(t *testing.T) {
	t.Parallel()
	mockResp := &domain.OptionsResponse{
		Commands: []domain.Command{
			{
				Command:          "ls -la",
				ShortExplanation: "List all files",
				IsDangerous:      false,
			},
		},
		IsValid: true,
	}

	mock := &mockProvider{
		name:     "test",
		model:    "test-model",
		response: mockResp,
	}

	srv := New(mock)
	resp, err := srv.GetOptions(context.Background(), "list files")
	if err != nil {
		t.Fatalf("GetOptions failed: %v", err)
	}

	if len(resp.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(resp.Commands))
	}
	if resp.Commands[0].Command != "ls -la" {
		t.Errorf("expected command='ls -la', got %s", resp.Commands[0].Command)
	}
}

func TestGetOptionsEmptyPrompt(t *testing.T) {
	t.Parallel()
	mock := &mockProvider{
		name:  "test",
		model: "test-model",
	}

	srv := New(mock)
	_, err := srv.GetOptions(context.Background(), "")
	if !errors.Is(err, domain.ErrInvalidPrompt) {
		t.Errorf("expected ErrInvalidPrompt, got %v", err)
	}
}

func TestGetEnvContext(t *testing.T) {
	t.Parallel()
	mock := &mockProvider{
		name:  "test",
		model: "test-model",
	}

	srv := New(mock)
	ctx := srv.getEnvContext()

	if ctx == "" {
		t.Error("expected non-empty env context")
	}
	// Проверяем, что содержит OS
	if len(ctx) < 5 {
		t.Errorf("expected env context to contain OS info, got: %s", ctx)
	}
}
