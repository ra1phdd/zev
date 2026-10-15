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

	model, err := prompt.Input("Enter the model to use (default: gpt-5.6-luna):")
	if err != nil {
		return err
	}
	if model != "" {
		cfg.OpenAIModel = model
	}

	baseURL, err := prompt.Input("Enter custom base URL (press Enter for default):")
	if err != nil {
		return err
	}
	if baseURL != "" {
		cfg.OpenAIBaseURL = baseURL
	}

	return nil
}

func (s *Service) setupGemini(cfg *config.Config) error {
	apiKey, err := prompt.Input("Enter your Gemini API key:")
	if err != nil {
		return err
	}
	cfg.GeminiAPIKey = apiKey

	model, err := prompt.Input("Enter the model to use (default: gemini-3.1-flash-lite):")
	if err != nil {
		return err
	}
	if model != "" {
		cfg.GeminiModel = model
	}

	return nil
}

func (s *Service) setupOllama(cfg *config.Config) error {
	baseURL, err := prompt.Input("Enter Ollama base URL (default: http://localhost:11434):")
	if err != nil {
		return err
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	cfg.OllamaBaseURL = baseURL

	model, err := prompt.Input("Enter the model to use (e.g., llama2):")
	if err != nil {
		return err
	}
	cfg.OllamaModel = model

	return nil
}
