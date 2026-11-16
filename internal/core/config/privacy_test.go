package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config loading uses a test-local path and process environment.
func TestLoadDoesNotMutateEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := getConfigPath
	getConfigPath = func() string { return path }
	t.Cleanup(func() { getConfigPath = previous })
	t.Setenv("OPENAI_MODEL", "")
	if err := os.Unsetenv("OPENAI_MODEL"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"first-model", "second-model"} {
		if err := os.WriteFile(path, []byte("OPENAI_MODEL="+model+"\n"), 0o600); err != nil {
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
	if err != nil || cfg.OpenAIModel != "environment-model" {
		t.Fatalf("environment override = %+v, %v", cfg, err)
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestSaveQuotesValuesWithoutRecordInjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := getConfigPath
	getConfigPath = func() string { return path }
	t.Cleanup(func() { getConfigPath = previous })
	value := " key # \"quoted\" \\path $VARIABLE\nLLM_PROVIDER=attacker\r\nend "
	cfg := DefaultConfig()
	cfg.OpenAIAPIKey = value
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["OPENAI_API_KEY"] != value || values["LLM_PROVIDER"] != cfg.LLMProvider {
		t.Error("quoted value did not round-trip or injected a config record")
	}
}

// Config diagnostics must not reveal malformed file or environment values.
func TestConfigErrorsHideValues(t *testing.T) {
	const secret = "synthetic-private-secret"
	path := filepath.Join(t.TempDir(), "config")
	previous := getConfigPath
	getConfigPath = func() string { return path }
	t.Cleanup(func() { getConfigPath = previous })
	t.Setenv("HTTP_HEADERS", secret)
	_, err := Load()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe config parse error: %v", err)
	}
	if parseErr, ok := errors.AsType[env.ParseError](err); !ok || parseErr.Name == "" {
		t.Errorf("parse error identity lost: %T", err)
	}
	t.Setenv("HTTP_HEADERS", "X-Test:valid")
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
