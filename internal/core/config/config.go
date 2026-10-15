package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
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
	cfg := DefaultConfig()

	// Загружаем .env файл если он существует
	configPath := getConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		if err := godotenv.Load(configPath); err != nil {
			return nil, fmt.Errorf("failed to load .env: %w", err)
		}
	}

	// Парсим переменные окружения в структуру
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse env: %w", err)
	}

	return cfg, nil
}

// Save сохраняет конфигурацию в .env файл.
func Save(cfg *Config) error {
	configPath := getConfigPath()

	// Создаём директорию если её нет
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// Формируем содержимое .env через рефлексию
	var lines []string
	v := reflect.ValueOf(*cfg)
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		value := v.Field(i)
		lines = append(lines, fmt.Sprintf("%s=%v", envTag, value.Interface()))
	}

	content := strings.Join(lines, "\n") + "\n"

	// Записываем файл с правами 0600 (только владелец может читать/писать)
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// getConfigPath возвращает путь к конфигурационному файлу.
// Переменная для возможности подмены в тестах.
var getConfigPath = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zevrc")
}
