package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetZevDir(t *testing.T) {
	homeEnv := "HOME"
	switch runtime.GOOS {
	case "windows":
		homeEnv = "USERPROFILE"
	case "plan9":
		homeEnv = "home"
	}
	t.Run("home available", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv(homeEnv, home)
		if got, want := GetZevDir(), filepath.Join(home, ".zev"); got != want {
			t.Errorf("GetZevDir() = %q, want %q", got, want)
		}
	})
	t.Run("home unavailable", func(t *testing.T) {
		t.Setenv(homeEnv, "")
		if got := GetZevDir(); got != ".zev" {
			t.Errorf("GetZevDir() = %q, want .zev", got)
		}
	})
}

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

//nolint:paralleltest // Mutates the global config path.
func TestLoadConfigWithMissing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".zevrc")

	originalGetConfigPath := getConfigPath
	getConfigPath = func() string { return configPath }
	t.Cleanup(func() { getConfigPath = originalGetConfigPath })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LLMProvider != "openai" {
		t.Errorf("expected default provider, got %s", cfg.LLMProvider)
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".zevrc")

	originalGetConfigPath := getConfigPath
	getConfigPath = func() string { return configPath }
	t.Cleanup(func() { getConfigPath = originalGetConfigPath })

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

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file not created")
	}

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}

	if loadedCfg.LLMProvider != cfg.LLMProvider {
		t.Errorf("LLMProvider mismatch: got %s, want %s", loadedCfg.LLMProvider, cfg.LLMProvider)
	}
	if loadedCfg.GeminiAPIKey != cfg.GeminiAPIKey {
		t.Errorf("GeminiAPIKey mismatch: got %s, want %s", loadedCfg.GeminiAPIKey, cfg.GeminiAPIKey)
	}
	if loadedCfg.GeminiModel != cfg.GeminiModel {
		t.Errorf("GeminiModel mismatch: got %s, want %s", loadedCfg.GeminiModel, cfg.GeminiModel)
	}
}
