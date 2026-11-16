package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	testCommand  = "ls -la"
	testProvider = "test"
	testModel    = "test-model"
)

// mockProvider для тестов.
type mockProvider struct {
	name     string
	model    string
	response *domain.OptionsResponse
	err      error
}

func (m *mockProvider) GetOptions(_ context.Context, _ string, _ string) (*domain.OptionsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockProvider) GetOptionsWithHistory(_ context.Context, _ *domain.ConversationHistory) (*domain.OptionsResponse, error) {
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
				Command:          testCommand,
				ShortExplanation: "List all files",
				IsDangerous:      false,
			},
		},
		IsValid: true,
	}

	mock := &mockProvider{
		name:     testProvider,
		model:    testModel,
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
	if resp.Commands[0].Command != testCommand {
		t.Errorf("expected command='ls -la', got %s", resp.Commands[0].Command)
	}
}

func TestGetOptionsEmptyPrompt(t *testing.T) {
	t.Parallel()
	mock := &mockProvider{
		name:  testProvider,
		model: testModel,
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
		name:  testProvider,
		model: testModel,
	}

	srv := New(mock)
	ctx := srv.GetEnvContext()

	if ctx == "" {
		t.Error("expected non-empty env context")
	}
	// Проверяем, что содержит OS
	if len(ctx) < 5 {
		t.Errorf("expected env context to contain OS info, got: %s", ctx)
	}
}
