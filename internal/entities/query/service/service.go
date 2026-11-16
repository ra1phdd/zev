// Package service provides query business logic and local command safety checks.
package service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// Service управляет запросами к LLM провайдерам.
type Service struct {
	provider domain.InferenceProvider
}

// New creates a query service backed by provider.
func New(provider domain.InferenceProvider) *Service {
	return &Service{provider: provider}
}

// GetOptions requests command options for prompt and returns provider errors wrapped with context.
func (s *Service) GetOptions(ctx context.Context, prompt string) (*domain.OptionsResponse, error) {
	if prompt == "" {
		return nil, domain.ErrInvalidPrompt
	}

	envContext := s.GetEnvContext()
	response, err := s.provider.GetOptions(ctx, prompt, envContext)
	if err != nil {
		return nil, fmt.Errorf("get command options: %w", err)
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("validate command options: %w", err)
	}
	ApplyDangerousValidation(response)
	return response, nil
}

// GetOptionsWithHistory requests command options using an existing conversation history.
func (s *Service) GetOptionsWithHistory(ctx context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	response, err := s.provider.GetOptionsWithHistory(ctx, history)
	if err != nil {
		return nil, fmt.Errorf("get command options with history: %w", err)
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("validate command options: %w", err)
	}
	ApplyDangerousValidation(response)
	return response, nil
}

// GetEnvContext собирает информацию об окружении пользователя.
func (s *Service) GetEnvContext() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = os.Getenv("COMSPEC") // Windows
	}

	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME") // Windows
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE") // Windows
	}

	pwd, err := os.Getwd()
	if err != nil {
		pwd = ""
	}

	var result strings.Builder
	fmt.Fprintf(&result, "OS: %s\n", osName)
	fmt.Fprintf(&result, "Architecture: %s\n", arch)
	if shell != "" {
		fmt.Fprintf(&result, "Shell: %s\n", shell)
	}
	if user != "" {
		fmt.Fprintf(&result, "User: %s\n", user)
	}
	if home != "" {
		fmt.Fprintf(&result, "Home: %s\n", home)
	}
	if pwd != "" {
		fmt.Fprintf(&result, "Working Directory: %s", pwd)
	}

	return result.String()
}
