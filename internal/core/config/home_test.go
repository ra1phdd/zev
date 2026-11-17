package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Config operations resolve the process home before touching storage.
func TestConfigRejectsUntrustedHomeBeforeIO(t *testing.T) {
	for _, home := range []string{"", ".", "relative-home"} {
		for _, existing := range []bool{false, true} {
			name := home + "/absent"
			if existing {
				name = home + "/existing"
			}
			t.Run(name, func(t *testing.T) {
				project := t.TempDir()
				t.Chdir(project)
				t.Setenv("HOME", home)
				t.Setenv("USERPROFILE", home)
				t.Setenv("home", home)
				previous := SetConfigPathForTests("")
				t.Cleanup(func() { SetConfigPathForTests(previous) })
				dir := filepath.Join(project, home, ".zev")
				const original = "OPENAI_MODEL=untrusted-project-model\n"
				if existing {
					if err := os.MkdirAll(dir, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, "config"), []byte(original), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				called := false
				operations := map[string]func() error{
					"load":             func() error { _, err := Load(); return err },
					"load UI language": func() error { _, err := LoadUILanguage(); return err },
					"save":             func() error { return Save(DefaultConfig()) },
					"update": func() error {
						return Update(func(*Config) error { called = true; return nil })
					},
				}
				for operation, run := range operations {
					if err := run(); !errors.Is(err, ErrInvalidHome) {
						t.Errorf("%s error = %v, want ErrInvalidHome", operation, err)
					}
				}
				if called {
					t.Error("Update callback called for untrusted home")
				}
				entries, err := os.ReadDir(dir)
				if !existing {
					if !errors.Is(err, os.ErrNotExist) {
						t.Errorf("storage directory created: %v, %v", entries, err)
					}
					return
				}
				if err != nil || len(entries) != 1 || entries[0].Name() != "config" {
					t.Fatalf("project storage changed, including lock files: %v, %v", entries, err)
				}
				data, err := os.ReadFile(filepath.Join(dir, "config")) //nolint:gosec // Reads only this test's synthetic config fixture.
				if err != nil || string(data) != original {
					t.Errorf("project config changed: %q, %v", data, err)
				}
			})
		}
	}
}

// Test-only absolute overrides are independent of home resolution and nest safely.
func TestConfigPathOverrideWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")
	for _, key := range []string{"OPENAI_MODEL", toolApprovalModeEnv, historySemanticEnabledEnv} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	previous := SetConfigPathForTests("")
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	outer := filepath.Join(t.TempDir(), "config")
	if old := SetConfigPathForTests(outer); old != "" {
		t.Fatalf("previous override = %q, want default", old)
	}
	if err := Save(DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(t.TempDir(), "config")
	if old := SetConfigPathForTests(inner); old != outer {
		t.Fatalf("nested previous override = %q, want %q", old, outer)
	}
	if err := Update(func(cfg *Config) error { cfg.OpenAIModel = "override-model"; return nil }); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil || cfg.OpenAIModel != "override-model" {
		t.Fatalf("override load = %+v, %v", cfg, err)
	}
	SetConfigPathForTests(outer)
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	SetConfigPathForTests("")
	if _, err := Load(); !errors.Is(err, ErrInvalidHome) {
		t.Fatalf("restored default error = %v, want ErrInvalidHome", err)
	}
}
