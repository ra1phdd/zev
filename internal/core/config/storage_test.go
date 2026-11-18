package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:paralleltest // Mutates the global config path.
func TestConfigUpdateConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
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
	gotCfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := gotCfg.OpenAIModel, "start"+strings.Repeat("x", count); got != want {
		t.Errorf("model = %q, want %q", got, want)
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestConfigUpdatePersistsDisabledSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	if err := Update(func(cfg *Config) error {
		cfg.HistoryEnabled = false
		cfg.ContextEnabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	gotCfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if gotCfg.HistoryEnabled || gotCfg.ContextEnabled {
		t.Fatal("disabled setting restored to true")
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestConfigUpdateFailurePreservesFile(t *testing.T) {
	for _, content := range []string{minimalYAML, unversionedYAML, "invalid [ config\n"} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
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
