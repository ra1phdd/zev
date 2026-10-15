package config

import (
	"os"
	"path/filepath"

	"github.com/ra1phdd/config"
)

// Config представляет конфигурацию zev.
type Config struct {
	// Общие настройки
	LLMProvider string `env:"LLM_PROVIDER"`

	// OpenAI (и OpenAI-compatible)
	OpenAIAPIKey  string `env:"OPENAI_API_KEY"`
	OpenAIModel   string `env:"OPENAI_MODEL"`
	OpenAIBaseURL string `env:"OPENAI_BASE_URL"`

	// Gemini
	GeminiAPIKey string `env:"GEMINI_API_KEY"`
	GeminiModel  string `env:"GEMINI_MODEL"`

	// Ollama
	OllamaBaseURL string `env:"OLLAMA_BASE_URL"`
	OllamaModel   string `env:"OLLAMA_MODEL"`
}

// DefaultConfig возвращает конфигурацию с дефолтными значениями.
func DefaultConfig() *Config {
	return &Config{
		LLMProvider:   "openai",
		OpenAIModel:   "gpt-5.6-luna",
		OpenAIBaseURL: "https://api.openai.com/v1",
		GeminiModel:   "gemini-3.1-flash-lite",
	}
}

// Load загружает конфигурацию из .env файла.
func Load() (*Config, error) {
	cfg, err := config.LoadGeneric(
		DefaultConfig,
		config.WithConfigPath(getConfigPath()),
		config.WithSecurityPath(getSecurityPath()),
		config.WithEnvironment(true),
		config.WithMissingConfigAllowed(true),
		config.WithEmptyConfigAllowed(true),
	)
	return cfg, err
}

// Save сохраняет конфигурацию в .env файл.
func Save(cfg *Config) error {
	return config.SaveGeneric(
		cfg,
		config.WithConfigPath(getConfigPath()),
		config.WithSecurityPath(getSecurityPath()),
	)
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zevrc")
}

func getSecurityPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zevrc.security")
}
