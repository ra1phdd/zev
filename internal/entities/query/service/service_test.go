package service

import (
	"context"
	"errors"
	"runtime"
	"strings"
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
	name       string
	model      string
	response   *domain.OptionsResponse
	err        error
	prompt     string
	envContext string
}

func (m *mockProvider) GetOptions(_ context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	m.prompt = prompt
	m.envContext = envContext
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
			{
				Command:     "rm -rf ./tmp",
				IsDangerous: false,
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

	if mock.prompt != "list files" || mock.envContext == "" {
		t.Fatalf("provider request prompt=%q env=%q", mock.prompt, mock.envContext)
	}
	if len(resp.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(resp.Commands))
	}
	if resp.Commands[0].Command != testCommand {
		t.Errorf("expected command='ls -la', got %s", resp.Commands[0].Command)
	}
	if !resp.Commands[1].IsDangerous || resp.Commands[1].DangerousExplanation == "" {
		t.Fatalf("local safety validation did not mark dangerous command: %+v", resp.Commands[1])
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
	if !strings.Contains(ctx, "OS: "+runtime.GOOS) || !strings.Contains(ctx, "Architecture: "+runtime.GOARCH) {
		t.Fatalf("expected env context to contain OS and architecture, got: %s", ctx)
	}
}
