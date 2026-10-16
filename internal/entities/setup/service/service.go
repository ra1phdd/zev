package service

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
)

type Service struct{}

func New() *Service {
	return &Service{}
}

// maskAPIKey маскирует API ключ, показывая первые 3 и последние 3 символа.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 6 {
		return "***"
	}
	return key[:3] + "***..." + key[len(key)-3:]
}

// promptWithDefault показывает промпт с текущим значением и дефолтом.
// Если пользователь жмёт Enter — возвращает current.
// Если пользователь пишет "default" — возвращает defaultVal.
// Иначе возвращает то, что пользователь ввёл.
func promptWithDefault(promptText, current, defaultVal string) (string, error) {
	var fullPrompt string
	switch {
	case current != "" && defaultVal != "" && current != defaultVal:
		fullPrompt = fmt.Sprintf("%s (current: %s, press Enter to keep or type 'default' for %s):", promptText, current, defaultVal)
	case current != "":
		fullPrompt = fmt.Sprintf("%s (current: %s, press Enter to keep):", promptText, current)
	case defaultVal != "":
		fullPrompt = fmt.Sprintf("%s (default: %s):", promptText, defaultVal)
	default:
		fullPrompt = promptText + ":"
	}

	input, err := prompt.Input(fullPrompt)
	if err != nil {
		return "", err
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

func (s *Service) Run() error {
	// Загружаем текущий конфиг (или дефолтный, если файла нет)
	cfg, err := config.Load()
	if err != nil {
		// Если файл не существует, используем дефолт
		cfg = config.DefaultConfig()
	}

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
	defaults := config.DefaultConfig()

	// API Key с маскировкой
	currentMasked := ""
	if cfg.OpenAIAPIKey != "" {
		currentMasked = maskAPIKey(cfg.OpenAIAPIKey)
	}
	apiKeyPrompt := "Enter your OpenAI API key"
	if currentMasked != "" {
		apiKeyPrompt = fmt.Sprintf("%s (current: %s, press Enter to keep)", apiKeyPrompt, currentMasked)
	}
	apiKey, err := prompt.Input(apiKeyPrompt + ":")
	if err != nil {
		return err
	}
	if apiKey == "" && cfg.OpenAIAPIKey != "" {
		// Оставляем текущий ключ
	} else if apiKey != "" {
		cfg.OpenAIAPIKey = apiKey
	}

	// Model
	model, err := promptWithDefault("Enter the model to use", cfg.OpenAIModel, defaults.OpenAIModel)
	if err != nil {
		return err
	}
	cfg.OpenAIModel = model

	// Base URL
	baseURL, err := promptWithDefault("Enter custom base URL", cfg.OpenAIBaseURL, defaults.OpenAIBaseURL)
	if err != nil {
		return err
	}
	cfg.OpenAIBaseURL = baseURL

	return nil
}

func (s *Service) setupGemini(cfg *config.Config) error {
	defaults := config.DefaultConfig()

	// API Key с маскировкой
	currentMasked := ""
	if cfg.GeminiAPIKey != "" {
		currentMasked = maskAPIKey(cfg.GeminiAPIKey)
	}
	apiKeyPrompt := "Enter your Gemini API key"
	if currentMasked != "" {
		apiKeyPrompt = fmt.Sprintf("%s (current: %s, press Enter to keep)", apiKeyPrompt, currentMasked)
	}
	apiKey, err := prompt.Input(apiKeyPrompt + ":")
	if err != nil {
		return err
	}
	if apiKey == "" && cfg.GeminiAPIKey != "" {
		// Оставляем текущий ключ
	} else if apiKey != "" {
		cfg.GeminiAPIKey = apiKey
	}

	// Model
	model, err := promptWithDefault("Enter the model to use", cfg.GeminiModel, defaults.GeminiModel)
	if err != nil {
		return err
	}
	cfg.GeminiModel = model

	// Base URL
	baseURL, err := promptWithDefault("Enter custom base URL", cfg.GeminiBaseURL, defaults.GeminiBaseURL)
	if err != nil {
		return err
	}
	cfg.GeminiBaseURL = baseURL

	return nil
}

func (s *Service) setupAnthropic(cfg *config.Config) error {
	defaults := config.DefaultConfig()

	// API Key с маскировкой
	currentMasked := ""
	if cfg.AnthropicAPIKey != "" {
		currentMasked = maskAPIKey(cfg.AnthropicAPIKey)
	}
	apiKeyPrompt := "Enter your Anthropic API key"
	if currentMasked != "" {
		apiKeyPrompt = fmt.Sprintf("%s (current: %s, press Enter to keep)", apiKeyPrompt, currentMasked)
	}
	apiKey, err := prompt.Input(apiKeyPrompt + ":")
	if err != nil {
		return err
	}
	if apiKey == "" && cfg.AnthropicAPIKey != "" {
		// Оставляем текущий ключ
	} else if apiKey != "" {
		cfg.AnthropicAPIKey = apiKey
	}

	// Model
	model, err := promptWithDefault("Enter the model to use", cfg.AnthropicModel, defaults.AnthropicModel)
	if err != nil {
		return err
	}
	cfg.AnthropicModel = model

	// Base URL
	baseURL, err := promptWithDefault("Enter custom base URL", cfg.AnthropicBaseURL, defaults.AnthropicBaseURL)
	if err != nil {
		return err
	}
	cfg.AnthropicBaseURL = baseURL

	return nil
}

func (s *Service) setupOllama(cfg *config.Config) error {
	defaults := config.DefaultConfig()

	// Base URL
	baseURL, err := promptWithDefault("Enter Ollama base URL", cfg.OllamaBaseURL, defaults.OllamaBaseURL)
	if err != nil {
		return err
	}
	cfg.OllamaBaseURL = baseURL

	// Model
	model, err := promptWithDefault("Enter the model to use", cfg.OllamaModel, "")
	if err != nil {
		return err
	}
	cfg.OllamaModel = model

	return nil
}

func (s *Service) setupOpenCode(cfg *config.Config) error {
	// API Key с маскировкой
	currentMasked := ""
	if cfg.OpenAIAPIKey != "" {
		currentMasked = maskAPIKey(cfg.OpenAIAPIKey)
	}
	apiKeyPrompt := "Enter your OpenCode API key"
	if currentMasked != "" {
		apiKeyPrompt = fmt.Sprintf("%s (current: %s, press Enter to keep)", apiKeyPrompt, currentMasked)
	}
	apiKey, err := prompt.Input(apiKeyPrompt + ":")
	if err != nil {
		return err
	}
	if apiKey == "" && cfg.OpenAIAPIKey != "" {
		// Оставляем текущий ключ
	} else if apiKey != "" {
		cfg.OpenAIAPIKey = apiKey
	}

	// Model
	currentModel := cfg.OpenAIModel
	if currentModel == "" {
		currentModel = "big-pickle"
	}
	model, err := promptWithDefault("Enter the model to use", currentModel, "big-pickle")
	if err != nil {
		return err
	}
	cfg.OpenAIModel = model

	// Base URL фиксированный для OpenCode
	cfg.OpenAIBaseURL = "https://opencode.ai/inference/openai/v1"

	// HTTP Headers фиксированные для OpenCode
	cfg.HTTPHeaders = map[string]string{
		"User-Agent":         "opencode/latest/2.0.29/cli",
		"x-opencode-session": "ses_ebeea89daffe69kTavLoPHLIzF",
	}

	return nil
}
