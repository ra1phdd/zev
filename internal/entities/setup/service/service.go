package service

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/pkg/tui/list"
	"github.com/ra1phdd/zev-go/pkg/tui/prompt"
)

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) Run() error {
	cfg := config.DefaultConfig()

	providers := []string{"openai", "gemini", "anthropic", "ollama", "opencode"}
	items := make([]list.Item, len(providers))
	for i, p := range providers {
		items[i] = list.NewSimpleItem(p, "")
	}

	selected, err := list.Select(list.SelectOptions{
		Title: "Which LLM provider would you like to use?",
		Items: items,
	})
	if err != nil {
		return fmt.Errorf("failed to select provider: %w", err)
	}

	provider := selected.(list.SimpleItem).Title()
	cfg.LLMProvider = provider

	switch provider {
	case "openai":
		if err := s.setupOpenAI(cfg); err != nil {
			return err
		}
	case "gemini":
		if err := s.setupGemini(cfg); err != nil {
			return err
		}
	case "anthropic":
		if err := s.setupAnthropic(cfg); err != nil {
			return err
		}
	case "ollama":
		if err := s.setupOllama(cfg); err != nil {
			return err
		}
	case "opencode":
		if err := s.setupOpenCode(cfg); err != nil {
			return err
		}
	}

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

	baseURL, err := prompt.Input("Enter custom base URL (default: https://generativelanguage.googleapis.com):")
	if err != nil {
		return err
	}
	if baseURL != "" {
		cfg.GeminiBaseURL = baseURL
	}

	return nil
}

func (s *Service) setupAnthropic(cfg *config.Config) error {
	apiKey, err := prompt.Input("Enter your Anthropic API key:")
	if err != nil {
		return err
	}
	cfg.AnthropicAPIKey = apiKey

	model, err := prompt.Input("Enter the model to use (default: claude-haiku-5.5):")
	if err != nil {
		return err
	}
	if model != "" {
		cfg.AnthropicModel = model
	}

	baseURL, err := prompt.Input("Enter custom base URL (default: https://api.anthropic.com/v1):")
	if err != nil {
		return err
	}
	if baseURL != "" {
		cfg.AnthropicBaseURL = baseURL
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

func (s *Service) setupOpenCode(cfg *config.Config) error {
	apiKey, err := prompt.Input("Enter your OpenCode API key:")
	if err != nil {
		return err
	}
	cfg.OpenAIAPIKey = apiKey

	model, err := prompt.Input("Enter the model to use (default: big-pickle):")
	if err != nil {
		return err
	}
	if model == "" {
		model = "big-pickle"
	}
	cfg.OpenAIModel = model

	cfg.OpenAIBaseURL = "https://opencode.ai/inference/openai/v1"

	cfg.HTTPHeaders = map[string]string{
		"User-Agent":         "opencode/latest/2.0.29/cli",
		"x-opencode-session": "ses_ebeea89daffe69kTavLoPHLIzF",
	}

	return nil
}
