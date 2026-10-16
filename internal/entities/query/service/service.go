package service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/ra1phdd/zev-go/internal/core/domain"
)

// Service управляет запросами к LLM провайдерам.
type Service struct {
	provider domain.InferenceProvider
}

func New(provider domain.InferenceProvider) *Service {
	return &Service{provider: provider}
}

func (s *Service) GetOptions(ctx context.Context, prompt string) (*domain.OptionsResponse, error) {
	if prompt == "" {
		return nil, domain.ErrInvalidPrompt
	}

	envContext := s.GetEnvContext()
	return s.provider.GetOptions(ctx, prompt, envContext)
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

	pwd, _ := os.Getwd()

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
