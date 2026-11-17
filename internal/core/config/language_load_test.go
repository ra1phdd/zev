package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	languageRussianFixture = "ZEV_LANG=ru\n"
	languageSpacedEnglish  = " EN "
	languageSpacedRussian  = " RU "
)

func TestLoadUILanguageReadOnlyAndPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, file, environment, want string
		setEnvironment, existing      bool
	}{
		{name: "missing directory", want: uiLanguageAuto},
		{name: "file", file: languageRussianFixture, existing: true, want: "ru"},
		{name: "unrelated invalid settings ignored", file: "ZEV_LANG=en\nHISTORY_SEMANTIC_ENABLED=true\nTOOL_APPROVAL_MODE=bad\nHTTP_HEADERS=bad\n", existing: true, want: "en"},
		{name: "environment", file: languageRussianFixture, existing: true, setEnvironment: true, environment: languageSpacedEnglish, want: "en"},
		{name: "auto environment", file: languageRussianFixture, existing: true, setEnvironment: true, environment: uiLanguageAuto, want: uiLanguageAuto},
		{name: "empty environment preserves file", file: languageRussianFixture, existing: true, setEnvironment: true, want: "ru"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".zev")
			path := filepath.Join(dir, "config")
			previous := SetConfigPathForTests(path)
			t.Cleanup(func() { SetConfigPathForTests(previous) })
			t.Setenv("ZEV_LANG", tc.environment)
			if !tc.setEnvironment {
				if err := os.Unsetenv("ZEV_LANG"); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HISTORY_SEMANTIC_ENABLED", "not-a-bool")
			t.Setenv("HTTP_HEADERS", "bad")
			if tc.existing {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := LoadUILanguage()
			if err != nil || got != tc.want {
				t.Fatalf("LoadUILanguage()=%q, %v; want %q", got, err, tc.want)
			}
			if value, exists := os.LookupEnv("ZEV_LANG"); value != tc.environment || exists != tc.setEnvironment {
				t.Errorf("environment mutated: value=%q, exists=%v", value, exists)
			}
			entries, err := os.ReadDir(dir)
			if !tc.existing {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("UI load created storage: %v %v", entries, err)
				}
				return
			}
			if err != nil || len(entries) != 1 || entries[0].Name() != "config" {
				t.Fatalf("UI load created sidecars: %v %v", entries, err)
			}
			data, err := os.ReadFile(filepath.Clean(path))
			if err != nil || string(data) != tc.file {
				t.Fatalf("config changed: error=%v", err)
			}
		})
	}
}

func TestLoadUILanguageErrorsHideValues(t *testing.T) {
	const secret = "synthetic-private-language"
	for _, content := range []string{"ZEV_LANG=" + secret + "\n", "invalid [ " + secret + "\n"} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			previous := SetConfigPathForTests(path)
			t.Cleanup(func() { SetConfigPathForTests(previous) })
			t.Setenv("ZEV_LANG", "")
			if err := os.Unsetenv("ZEV_LANG"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadUILanguage(); err == nil || strings.Contains(err.Error(), secret) {
				t.Fatalf("unsafe or absent language load error: %v", err)
			}
		})
	}
	t.Run("environment", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config")
		previous := SetConfigPathForTests(path)
		t.Cleanup(func() { SetConfigPathForTests(previous) })
		t.Setenv("ZEV_LANG", secret)
		if _, err := LoadUILanguage(); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("unsafe or absent environment error: %v", err)
		}
	})
}
