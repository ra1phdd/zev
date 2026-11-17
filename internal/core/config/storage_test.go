package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

//nolint:paralleltest // Mutates the global config path.
func TestConfigUpdateConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	if err := Save(&Config{OpenAIModel: "start", ToolApprovalMode: toolApprovalManual}); err != nil {
		t.Fatal(err)
	}
	const count = 40
	errs := make(chan error, count)
	for range count {
		go func() {
			errs <- Update(func(cfg *Config) error {
				cfg.OpenAIModel += "x"
				return nil
			})
		}()
	}
	for range count {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := values["OPENAI_MODEL"], "start"+strings.Repeat("x", count); got != want {
		t.Errorf("model = %q, want %q", got, want)
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestConfigUpdatePersistsDisabledSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	if err := Update(func(cfg *Config) error {
		cfg.HistoryEnabled = false
		cfg.ContextEnabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	values, err := godotenv.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"HISTORY_ENABLED", "CONTEXT_ENABLED"} {
		if values[key] != configFalseValue {
			t.Errorf("%s = %q, want false", key, values[key])
		}
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestConfigUpdateFailurePreservesFile(t *testing.T) {
	for _, content := range []string{"OPENAI_MODEL=original\n", "invalid [ config\n"} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			previous := SetConfigPathForTests(path)
			t.Cleanup(func() { SetConfigPathForTests(previous) })
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("callback failure")
			if err := Update(func(cfg *Config) error { cfg.OpenAIModel = "changed"; return failure }); err == nil {
				t.Fatal("Update succeeded")
			}
			data, err := os.ReadFile(path) //nolint:gosec // Test path is created in t.TempDir.
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, []byte(content)) {
				t.Errorf("config changed on failure: %q", data)
			}
		})
	}
}
