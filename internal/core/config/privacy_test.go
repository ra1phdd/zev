package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Config loading uses a test-local path and process environment.
func TestLoadDoesNotMutateEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	t.Setenv("OPENAI_MODEL", "")
	if err := os.Unsetenv("OPENAI_MODEL"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"first-model", "second-model"} {
		if err := os.WriteFile(path, []byte("version: 1\nactive_provider: work\nproviders: {work: {type: openai, model: "+model+"}}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.OpenAIModel != model {
			t.Errorf("model = %q, want %q", cfg.OpenAIModel, model)
		}
		if _, exists := os.LookupEnv("OPENAI_MODEL"); exists {
			t.Error("Load mutated the process environment")
		}
	}
	t.Setenv("OPENAI_MODEL", "environment-model")
	cfg, err := Load()
	if err != nil || cfg.OpenAIModel != "second-model" {
		t.Fatalf("environment override = %+v, %v", cfg, err)
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestSaveQuotesValuesWithoutRecordInjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	value := " key # \"quoted\" \\path $VARIABLE\nLLM_PROVIDER=attacker\r\nend "
	cfg := DefaultConfig()
	cfg.OpenAIAPIKey = value
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	values, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if values.OpenAIAPIKey != value || values.LLMProvider != cfg.LLMProvider {
		t.Error("quoted value did not round-trip or injected a config record")
	}
}

// Config diagnostics must not reveal malformed file or environment values.
func TestConfigErrorsHideValues(t *testing.T) {
	const secret = "synthetic-private-secret"
	path := filepath.Join(t.TempDir(), "config.yaml")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	t.Setenv("HTTP_HEADERS", secret)
	if err := os.WriteFile(path, []byte("invalid [ "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { _, err := Load(); return err },
		func() error { return Update(func(*Config) error { return nil }) },
	} {
		if err := operation(); err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("unsafe malformed-file error: %v", err)
		}
	}
}
