package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	if cfg.LLMProvider != "openai" {
		t.Errorf("expected default provider=openai, got %s", cfg.LLMProvider)
	}
	if cfg.OpenAIModel != "gpt-5.6-luna" {
		t.Errorf("expected default model=gpt-5.6-luna, got %s", cfg.OpenAIModel)
	}
	if cfg.GeminiModel != "gemini-3.1-flash-lite" {
		t.Errorf("expected default gemini model=gemini-3.1-flash-lite, got %s", cfg.GeminiModel)
	}
	if cfg.OpenAIBaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default base_url=https://api.openai.com/v1, got %s", cfg.OpenAIBaseURL)
	}
}

func TestLoadConfigWithMissing(t *testing.T) {
	// Создаём временный каталог
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	securityPath := filepath.Join(tmpDir, "test.security.yaml")

	// Устанавливаем env для переопределения путей
	t.Setenv("APP_CONFIG", configPath)
	t.Setenv("APP_SECURITY", securityPath)

	// Загрузка должна пройти успешно с defaults
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LLMProvider != "openai" {
		t.Errorf("expected default provider, got %s", cfg.LLMProvider)
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	securityPath := filepath.Join(tmpDir, "config.security.yaml")

	t.Setenv("APP_CONFIG", configPath)
	t.Setenv("APP_SECURITY", securityPath)

	cfg := &Config{
		LLMProvider:   "gemini",
		GeminiAPIKey:  "test-key",
		GeminiModel:   "gemini-3.1-flash-lite",
		OpenAIModel:   "gpt-5.6-luna",
		OpenAIBaseURL: "https://api.openai.com/v1",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Проверяем, что файл создался
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file not created")
	}
}
