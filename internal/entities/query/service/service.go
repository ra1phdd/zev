package service

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/ra1phdd/zev-go/internal/core/domain"
)

// Service управляет запросами к LLM провайдерам.
type Service struct {
	provider domain.InferenceProvider
}

// New создаёт новый query service.
func New(provider domain.InferenceProvider) *Service {
	return &Service{provider: provider}
}

// GetOptions получает варианты команд от LLM провайдера.
func (s *Service) GetOptions(ctx context.Context, prompt string) (*domain.OptionsResponse, error) {
	if prompt == "" {
		return nil, domain.ErrInvalidPrompt
	}

	envContext := s.GetEnvContext()
	return s.provider.GetOptions(ctx, prompt, envContext)
}

// GetEnvContext собирает информацию об окружении пользователя.
func (s *Service) GetEnvContext() string {
	osName := runtime.GOOS + " " + runtime.GOARCH
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = os.Getenv("COMSPEC") // Windows
	}

	if shell != "" {
		return fmt.Sprintf("OS: %s\nSHELL: %s", osName, shell)
	}
	return "OS: " + osName
}
