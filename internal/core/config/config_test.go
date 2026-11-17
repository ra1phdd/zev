package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	toolApprovalModeEnv       = "TOOL_APPROVAL_MODE"
	historySemanticEnabledEnv = "HISTORY_SEMANTIC_ENABLED"
	historyEmbeddingProvider  = "openai-compatible"
	historyEmbeddingBaseURL   = "https://embedding.example/v1"
	toolApprovalManual        = "manual"
	toolApprovalAuto          = "auto"
	toolApprovalUnsupported   = "unsupported"
	configFalseValue          = "false"
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
		got, err := GetZevDir()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, ".zev"); got != want {
			t.Errorf("GetZevDir() = %q, want %q", got, want)
		}
	})
	for _, home := range []string{"", ".", "relative-home", "../parent-home"} {
		t.Run("untrusted home="+home, func(t *testing.T) {
			t.Setenv(homeEnv, home)
			got, err := GetZevDir()
			if got != "" || !errors.Is(err, ErrInvalidHome) {
				t.Errorf("GetZevDir() = %q, %v; want empty path and ErrInvalidHome", got, err)
			}
		})
	}
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

func TestDefaultToolConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	if !cfg.ToolsEnabled {
		t.Error("tools must be enabled by default")
	}
	if cfg.ToolApprovalMode != toolApprovalManual {
		t.Errorf("tool approval mode = %q, want %s", cfg.ToolApprovalMode, toolApprovalManual)
	}
}

func TestToolConfigEnvFilePrecedenceAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })

	want := DefaultConfig()
	want.ToolsEnabled = false
	want.ToolApprovalMode = toolApprovalAuto
	if err := Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	for _, key := range []string{"TOOLS_ENABLED", toolApprovalModeEnv} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load after Save failed: %v", err)
	}
	if got.ToolsEnabled != want.ToolsEnabled || got.ToolApprovalMode != want.ToolApprovalMode {
		t.Fatalf("tool config did not round-trip: got enabled=%v mode=%q", got.ToolsEnabled, got.ToolApprovalMode)
	}

	t.Setenv("TOOLS_ENABLED", "true")
	t.Setenv(toolApprovalModeEnv, "bypass")
	got, err = Load()
	if err != nil {
		t.Fatalf("Load with environment overrides failed: %v", err)
	}
	if !got.ToolsEnabled || got.ToolApprovalMode != "bypass" {
		t.Fatalf("environment did not override file: got enabled=%v mode=%q", got.ToolsEnabled, got.ToolApprovalMode)
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestToolApprovalModeValidationIsAlwaysApplied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	for _, mode := range []string{"", toolApprovalUnsupported, "AUTO"} {
		t.Run(mode, func(t *testing.T) {
			_, err := parseValues(map[string]string{
				"TOOLS_ENABLED":           configFalseValue,
				toolApprovalModeEnv:       mode,
				historySemanticEnabledEnv: configFalseValue,
			})
			if err == nil {
				t.Fatal("invalid tool approval mode was accepted")
			}
			if !strings.Contains(err.Error(), "invalid tool policy") || strings.Contains(err.Error(), "history embedding") {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}

	cfg := DefaultConfig()
	cfg.ToolApprovalMode = toolApprovalUnsupported
	if err := Save(cfg); err == nil || !strings.Contains(err.Error(), "invalid tool policy") {
		t.Fatalf("Save accepted invalid tool approval mode: %v", err)
	}

	if err := os.WriteFile(path, []byte("TOOL_APPROVAL_MODE="+toolApprovalUnsupported+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "invalid tool policy") {
		t.Fatalf("Load accepted invalid tool approval mode: %v", err)
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
		historySemanticEnabledEnv:      "true",
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
		"HISTORY_EMBEDDING_PROVIDER":   toolApprovalUnsupported,
		"HISTORY_EMBEDDING_BASE_URL":   "",
		"HISTORY_EMBEDDING_MODEL":      "",
		"HISTORY_EMBEDDING_DIMENSIONS": "0",
		"HISTORY_EMBEDDING_MAX_BYTES":  "0",
	}
	if _, err := parseValues(invalid); err != nil {
		t.Fatalf("disabled invalid embedding config rejected: %v", err)
	}
	invalid[historySemanticEnabledEnv] = "true"
	if _, err := parseValues(invalid); err == nil {
		t.Fatal("enabled invalid embedding config accepted")
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestLoadConfigWithMissing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".zevrc")

	originalGetConfigPath := SetConfigPathForTests(configPath)
	t.Cleanup(func() { SetConfigPathForTests(originalGetConfigPath) })

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

	originalGetConfigPath := SetConfigPathForTests(configPath)
	t.Cleanup(func() { SetConfigPathForTests(originalGetConfigPath) })

	cfg := &Config{
		LLMProvider:      "gemini",
		GeminiAPIKey:     "test-key",
		GeminiModel:      "gemini-3.1-flash-lite",
		OpenAIModel:      "gpt-5.6-luna",
		OpenAIBaseURL:    "https://api.openai.com/v1",
		ToolApprovalMode: toolApprovalManual,
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
	previous := SetConfigPathForTests(configPath)
	t.Cleanup(func() { SetConfigPathForTests(previous) })

	want := &Config{
		HistorySemanticEnabled:     true,
		ToolApprovalMode:           toolApprovalManual,
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
