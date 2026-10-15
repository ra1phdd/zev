package service

import (
	"crypto/rand"
	"encoding/hex"
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
		[]string{"openai", "gemini", "ollama", "opencode"},
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
	case "opencode":
		if err := s.setupOpenCode(cfg); err != nil {
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

func (s *Service) setupOpenCode(cfg *config.Config) error {
	apiKey, err := prompt.Input("Enter your OpenCode API key:")
	if err != nil {
		return err
	}
	cfg.OpenAIAPIKey = apiKey

	model, err := prompt.Input("Enter the model to use (default: claude-sonnet-4-20250514):")
	if err != nil {
		return err
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	cfg.OpenAIModel = model

	// Устанавливаем base URL для OpenCode
	cfg.OpenAIBaseURL = "https://opencode.ai/inference/openai/v1"

	// Генерируем случайные идентификаторы
	orgID := generateRandomID("wrk", 26)
	projectID := generateRandomHex(32)
	sessionID := generateRandomID("ses", 26)

	// Формируем заголовки OpenCode
	cfg.HTTPHeaders = map[string]string{
		"User-Agent":            "opencode/latest/2.0.29/cli",
		"x-opencode-client":     "cli",
		"x-opencode-org-id":     orgID,
		"x-opencode-project":    projectID,
		"x-opencode-session":    sessionID,
		"x-opencode-session-id": sessionID,
		"x-session-affinity":    sessionID,
		"x-session-id":          sessionID,
	}

	return nil
}

// generateRandomID генерирует случайный идентификатор с префиксом.
// Формат: <prefix>_<random_chars>.
func generateRandomID(prefix string, totalLength int) string {
	// totalLength включает префикс и underscore
	randomLength := totalLength - len(prefix) - 1
	if randomLength <= 0 {
		return prefix
	}

	bytes := make([]byte, randomLength/2+1) // hex будет в 2 раза длиннее
	if _, err := rand.Read(bytes); err != nil {
		// Fallback на константный ID при ошибке
		return fmt.Sprintf("%s_%s", prefix, "01KQGVDCYTW1M7393S4AE7QEE4")
	}

	hexStr := hex.EncodeToString(bytes)
	return fmt.Sprintf("%s_%s", prefix, hexStr[:randomLength])
}

// generateRandomHex генерирует случайную hex-строку заданной длины.
func generateRandomHex(length int) string {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback на константный hex при ошибке
		return "a41cb4feed8cc60a46a4fbb40c9fce37c74303a2"[:length]
	}

	hexStr := hex.EncodeToString(bytes)
	return hexStr[:length]
}
