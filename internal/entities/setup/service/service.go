// Package service implements the interactive configuration setup flow.
package service

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
)

const (
	providerOpenAI    = "openai"
	providerGemini    = "gemini"
	providerAnthropic = "anthropic"
	providerOllama    = "ollama"
	providerOpenCode  = "opencode"
)

// Service runs the interactive configuration setup flow.
type Service struct {
	runSecretInput func(tea.Model) (tea.Model, error)
	inputContext   func(context.Context, string) (string, error)
	selectContext  func(context.Context, list.SelectOptions) (list.Item, error)
}

// New creates a setup service.
func New() *Service {
	return &Service{inputContext: prompt.InputContext, selectContext: list.SelectContext}
}

// maskAPIKey conceals the entire saved key, including its length.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	return "***"
}

// promptWithDefault preserves current on Enter and accepts the literal "default"
// to restore defaultVal. Other input is returned unchanged.
func (s *Service) promptWithDefault(ctx context.Context, promptKey locale.Key, current, defaultVal string) (string, error) {
	promptText := locale.Text(promptKey)
	var fullPrompt string
	switch {
	case current != "" && defaultVal != "" && current != defaultVal:
		fullPrompt = fmt.Sprintf(locale.Text(locale.KeyUISetupCurrentDefault), promptText, current, defaultVal)
	case current != "":
		fullPrompt = fmt.Sprintf(locale.Text(locale.KeyUISetupCurrent), promptText, current)
	case defaultVal != "":
		fullPrompt = fmt.Sprintf(locale.Text(locale.KeyUISetupDefault), promptText, defaultVal)
	default:
		fullPrompt = promptText + ":"
	}

	input, err := s.inputContext(ctx, fullPrompt)
	if err != nil {
		return "", fmt.Errorf("read setup input: %w", err)
	}

	if input == "" {
		if current != "" {
			return current, nil
		}
		return defaultVal, nil
	}

	if input == "default" && defaultVal != "" {
		return defaultVal, nil
	}

	return input, nil
}

// RunContext prompts for provider settings and saves them unless ctx is canceled.
func (s *Service) RunContext(ctx context.Context) error {
	run := *s
	if run.inputContext == nil {
		run.inputContext = prompt.InputContext
	}
	if run.selectContext == nil {
		run.selectContext = list.SelectContext
	}
	s = &run
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup canceled: %w", err)
	}
	original, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg := *original

	providers := []string{providerOpenAI, providerGemini, providerAnthropic, providerOllama, providerOpenCode}
	items := make([]list.Item, len(providers))
	for i, p := range providers {
		items[i] = list.NewSimpleItem(p, "")
	}

	selected, err := s.selectContext(ctx, list.SelectOptions{
		Title: locale.Text(locale.KeyUISetupProvider),
		Items: items,
	})
	if err != nil {
		return fmt.Errorf("failed to select provider: %w", err)
	}
	item, ok := selected.(list.SimpleItem)
	if !ok {
		return fmt.Errorf("unexpected provider selection type %T", selected)
	}
	provider := item.Title()
	cfg.LLMProvider = provider

	switch provider {
	case providerOpenAI:
		if err := s.setupOpenAI(ctx, &cfg); err != nil {
			return err
		}
	case providerGemini:
		if err := s.setupGemini(ctx, &cfg); err != nil {
			return err
		}
	case providerAnthropic:
		if err := s.setupAnthropic(ctx, &cfg); err != nil {
			return err
		}
	case providerOllama:
		if err := s.setupOllama(ctx, &cfg); err != nil {
			return err
		}
	case providerOpenCode:
		if err := s.setupOpenCode(ctx, &cfg); err != nil {
			return err
		}
	}

	if err := config.Update(func(latest *config.Config) error {
		mergeProviderChanges(latest, original, &cfg, provider)
		return nil
	}); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if _, err := fmt.Fprintln(os.Stdout, locale.Text(locale.KeyUISetupSaved)); err != nil {
		return fmt.Errorf("failed to print success message: %w", err)
	}
	return nil
}

func (s *Service) setupOpenAI(ctx context.Context, cfg *config.Config) error {
	defaults := config.DefaultConfig()

	apiKey, err := s.promptAPIKey(ctx, "OpenAI", cfg.OpenAIAPIKey)
	if err != nil {
		return err
	}
	cfg.OpenAIAPIKey = apiKey

	// Model
	model, err := s.promptWithDefault(ctx, "Enter the model to use", cfg.OpenAIModel, defaults.OpenAIModel)
	if err != nil {
		return err
	}
	cfg.OpenAIModel = model

	// Base URL
	baseURL, err := s.promptWithDefault(ctx, "Enter custom base URL", cfg.OpenAIBaseURL, defaults.OpenAIBaseURL)
	if err != nil {
		return err
	}
	cfg.OpenAIBaseURL = baseURL

	return nil
}

func (s *Service) setupGemini(ctx context.Context, cfg *config.Config) error {
	defaults := config.DefaultConfig()

	apiKey, err := s.promptAPIKey(ctx, "Gemini", cfg.GeminiAPIKey)
	if err != nil {
		return err
	}
	cfg.GeminiAPIKey = apiKey

	// Model
	model, err := s.promptWithDefault(ctx, "Enter the model to use", cfg.GeminiModel, defaults.GeminiModel)
	if err != nil {
		return err
	}
	cfg.GeminiModel = model

	// Base URL
	baseURL, err := s.promptWithDefault(ctx, "Enter custom base URL", cfg.GeminiBaseURL, defaults.GeminiBaseURL)
	if err != nil {
		return err
	}
	cfg.GeminiBaseURL = baseURL

	return nil
}

func (s *Service) setupAnthropic(ctx context.Context, cfg *config.Config) error {
	defaults := config.DefaultConfig()

	apiKey, err := s.promptAPIKey(ctx, "Anthropic", cfg.AnthropicAPIKey)
	if err != nil {
		return err
	}
	cfg.AnthropicAPIKey = apiKey

	// Model
	model, err := s.promptWithDefault(ctx, "Enter the model to use", cfg.AnthropicModel, defaults.AnthropicModel)
	if err != nil {
		return err
	}
	cfg.AnthropicModel = model

	// Base URL
	baseURL, err := s.promptWithDefault(ctx, "Enter custom base URL", cfg.AnthropicBaseURL, defaults.AnthropicBaseURL)
	if err != nil {
		return err
	}
	cfg.AnthropicBaseURL = baseURL

	return nil
}

func (s *Service) setupOllama(ctx context.Context, cfg *config.Config) error {
	defaults := config.DefaultConfig()

	// Base URL
	baseURL, err := s.promptWithDefault(ctx, "Enter Ollama base URL", cfg.OllamaBaseURL, defaults.OllamaBaseURL)
	if err != nil {
		return err
	}
	cfg.OllamaBaseURL = baseURL

	// Model
	model, err := s.promptWithDefault(ctx, "Enter the model to use", cfg.OllamaModel, "")
	if err != nil {
		return err
	}
	cfg.OllamaModel = model

	return nil
}

func (s *Service) setupOpenCode(ctx context.Context, cfg *config.Config) error {
	apiKey, err := s.promptAPIKey(ctx, "OpenCode", cfg.OpenAIAPIKey)
	if err != nil {
		return err
	}
	cfg.OpenAIAPIKey = apiKey

	// Model
	currentModel := cfg.OpenAIModel
	if currentModel == "" {
		currentModel = "big-pickle"
	}
	model, err := s.promptWithDefault(ctx, "Enter the model to use", currentModel, "big-pickle")
	if err != nil {
		return err
	}
	cfg.OpenAIModel = model

	// OpenCode uses a fixed base URL.
	cfg.OpenAIBaseURL = "https://opencode.ai/inference/openai/v1"

	// OpenCode uses fixed HTTP headers.
	cfg.HTTPHeaders = map[string]string{
		"User-Agent":         "opencode/latest/2.0.29/cli",
		"x-opencode-session": "ses_ebeea89daffe69kTavLoPHLIzF",
	}

	return nil
}
