package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	historyEmbeddingProvider = "openai-compatible"
	historyEmbeddingBaseURL  = "https://embedding.example/v1"
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

func TestDefaultHistoryEmbeddingConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	if cfg.HistorySemanticEnabled {
		t.Error("history semantic embeddings must be disabled by default")
	}
	if cfg.HistoryEmbeddingProvider != "" || cfg.HistoryEmbeddingBaseURL != "" || cfg.HistoryEmbeddingModel != "" {
		t.Error("history embedding provider settings must be empty by default")
	}
	if cfg.HistoryEmbeddingDimensions != 0 || cfg.HistoryEmbeddingAPIKey != "" {
		t.Error("history embedding dimensions and key must be empty by default")
	}
	if cfg.HistoryEmbeddingMaxBytes != 32768 {
		t.Errorf("history embedding max bytes = %d, want 32768", cfg.HistoryEmbeddingMaxBytes)
	}
}

func TestParseHistoryEmbeddingConfig(t *testing.T) {
	t.Parallel()
	cfg, err := parseValues(map[string]string{
		"HISTORY_SEMANTIC_ENABLED":     "true",
		"HISTORY_EMBEDDING_PROVIDER":   historyEmbeddingProvider,
		"HISTORY_EMBEDDING_BASE_URL":   historyEmbeddingBaseURL,
		"HISTORY_EMBEDDING_MODEL":      "embed-model",
		"HISTORY_EMBEDDING_DIMENSIONS": "1536",
		"HISTORY_EMBEDDING_API_KEY":    "embedding-secret",
		"HISTORY_EMBEDDING_MAX_BYTES":  "4096",
	})
	if err != nil {
		t.Fatalf("parseValues failed: %v", err)
	}
	if !cfg.HistorySemanticEnabled || cfg.HistoryEmbeddingProvider != historyEmbeddingProvider || cfg.HistoryEmbeddingBaseURL != historyEmbeddingBaseURL || cfg.HistoryEmbeddingModel != "embed-model" || cfg.HistoryEmbeddingDimensions != 1536 || cfg.HistoryEmbeddingAPIKey != "embedding-secret" || cfg.HistoryEmbeddingMaxBytes != 4096 {
		t.Fatalf("unexpected history embedding config: %+v", cfg)
	}
}

func TestHistoryEmbeddingValidationOnlyWhenEnabled(t *testing.T) {
	t.Parallel()
	invalid := map[string]string{
		"HISTORY_EMBEDDING_PROVIDER":   "unsupported",
		"HISTORY_EMBEDDING_BASE_URL":   "",
		"HISTORY_EMBEDDING_MODEL":      "",
		"HISTORY_EMBEDDING_DIMENSIONS": "0",
		"HISTORY_EMBEDDING_MAX_BYTES":  "0",
	}
	if _, err := parseValues(invalid); err != nil {
		t.Fatalf("disabled invalid embedding config rejected: %v", err)
	}
	invalid["HISTORY_SEMANTIC_ENABLED"] = "true"
	if _, err := parseValues(invalid); err == nil {
		t.Fatal("enabled invalid embedding config accepted")
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

//nolint:paralleltest // Mutates the global config path.
func TestSaveConfigRoundTripsHistoryEmbedding(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config")
	previous := getConfigPath
	getConfigPath = func() string { return configPath }
	t.Cleanup(func() { getConfigPath = previous })

	want := &Config{
		HistorySemanticEnabled:     true,
		HistoryEmbeddingProvider:   historyEmbeddingProvider,
		HistoryEmbeddingBaseURL:    historyEmbeddingBaseURL,
		HistoryEmbeddingModel:      "embedding-model",
		HistoryEmbeddingDimensions: 1536,
		HistoryEmbeddingAPIKey:     "separate-secret",
		HistoryEmbeddingMaxBytes:   4096,
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	if got.HistorySemanticEnabled != want.HistorySemanticEnabled || got.HistoryEmbeddingProvider != want.HistoryEmbeddingProvider || got.HistoryEmbeddingBaseURL != want.HistoryEmbeddingBaseURL || got.HistoryEmbeddingModel != want.HistoryEmbeddingModel || got.HistoryEmbeddingDimensions != want.HistoryEmbeddingDimensions || got.HistoryEmbeddingAPIKey != want.HistoryEmbeddingAPIKey || got.HistoryEmbeddingMaxBytes != want.HistoryEmbeddingMaxBytes {
		t.Fatalf("history embedding config mismatch: got %+v, want %+v", got, want)
	}
}
