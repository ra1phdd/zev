package service

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/pkg/tui/prompt"
)

// Service управляет интерактивной настройкой конфигурации.
type Service struct{}

// New создаёт новый setup service.
func New() *Service {
	return &Service{}
}

// Run запускает интерактивный процесс настройки.
func (s *Service) Run() error {
	cfg := config.DefaultConfig()

	// Выбор провайдера
	provider, err := prompt.Select(
		"Which LLM provider would you like to use?",
		[]string{"openai", "gemini", "ollama"},
	)
	if err != nil {
		return fmt.Errorf("failed to select provider: %w", err)
	}
	cfg.LLMProvider = provider

	// Настройка выбранного провайдера
	switch provider {
	case "openai":
		if err := s.setupOpenAI(cfg); err != nil {
			return err
		}
	case "gemini":
		if err := s.setupGemini(cfg); err != nil {
			return err
		}
	case "ollama":
		if err := s.setupOllama(cfg); err != nil {
			return err
		}
	}

	// Сохранение конфигурации
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintln(os.Stdout, "\n✓ Configuration saved successfully!")
	return nil
}

func (s *Service) setupOpenAI(cfg *config.Config) error {
	apiKey, err := prompt.Input("Enter your OpenAI API key:")
	if err != nil {
		return err
	}
	cfg.OpenAIAPIKey = apiKey

	model, err := prompt.Input("Model (default: gpt-4-turbo):")
	if err != nil {
		return err
	}
	if model != "" {
		cfg.OpenAIModel = model
	}

	baseURL, err := prompt.Input("Base URL (default: https://api.openai.com/v1):")
	if err != nil {
		return err
	}
	if baseURL != "" {
		cfg.OpenAIBaseURL = baseURL
	}
	// Если не указан, используется дефолт из DefaultConfig

	return nil
}

func (s *Service) setupGemini(cfg *config.Config) error {
	apiKey, err := prompt.Input("Enter your Gemini API key:")
	if err != nil {
		return err
	}
	cfg.GeminiAPIKey = apiKey

	model, err := prompt.Input("Model (default: gemini-3-flash-preview):")
	if err != nil {
		return err
	}
	if model != "" {
		cfg.GeminiModel = model
	}

	return nil
}

func (s *Service) setupOllama(cfg *config.Config) error {
	baseURL, err := prompt.Input("Base URL (default: http://localhost:11434):")
	if err != nil {
		return err
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	cfg.OllamaBaseURL = baseURL

	model, err := prompt.Input("Model (e.g., llama2, mistral):")
	if err != nil {
		return err
	}
	cfg.OllamaModel = model

	return nil
}
