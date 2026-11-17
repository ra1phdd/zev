package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUILanguageDefaultsAndParsing(t *testing.T) {
	t.Parallel()
	if got := DefaultConfig().UILanguage; got != uiLanguageAuto {
		t.Fatalf("default UI language=%q, want auto", got)
	}
	for _, tc := range []struct {
		input, want string
	}{
		{uiLanguageAuto, uiLanguageAuto},
		{"en", "en"},
		{"ru", "ru"},
		{languageSpacedRussian, "ru"},
		{" AuTo ", uiLanguageAuto},
		{languageSpacedEnglish, "en"},
		{"", uiLanguageAuto},
		{"  ", uiLanguageAuto},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			cfg, err := parseValues(map[string]string{"ZEV_LANG": tc.input})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.UILanguage != tc.want {
				t.Fatalf("UI language=%q, want %q", cfg.UILanguage, tc.want)
			}
		})
	}
}

func TestUILanguageValidationHidesInvalidValues(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"fr", "ru_RU.UTF-8", "synthetic-private-value"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			cfg := DefaultConfig()
			cfg.UILanguage = value
			if err := cfg.Validate(); err == nil || strings.Contains(err.Error(), value) {
				t.Errorf("unsafe or missing validation error: %v", err)
			}
			_, err := parseValues(map[string]string{"ZEV_LANG": value})
			if err == nil || strings.Contains(err.Error(), value) || !strings.Contains(err.Error(), "invalid UI language") {
				t.Fatalf("unsafe or missing parse error: %v", err)
			}
			var safe configError
			if !errors.As(err, &safe) || errors.Unwrap(safe) == nil {
				t.Errorf("validation error lost safe wrapper or cause: %v", err)
			}
		})
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestUILanguageSaveAndUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	cfg := DefaultConfig()
	cfg.UILanguage = languageSpacedRussian
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	values, err := readValues(path)
	if err != nil || values["ZEV_LANG"] != "ru" {
		t.Fatalf("saved language=%q, error=%v", values["ZEV_LANG"], err)
	}
	if cfg.UILanguage != languageSpacedRussian {
		t.Fatal("Save mutated caller configuration")
	}
	original, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	cfg.UILanguage = "synthetic-invalid-value"
	if err := Save(cfg); err == nil || strings.Contains(err.Error(), cfg.UILanguage) {
		t.Fatalf("unsafe or missing save error: %v", err)
	}
	if err := Update(func(cfg *Config) error { cfg.UILanguage = "bad"; return nil }); err == nil {
		t.Fatal("Update accepted invalid UI language")
	}
	after, err := os.ReadFile(filepath.Clean(path))
	if err != nil || string(after) != string(original) {
		t.Fatalf("invalid write changed config: error=%v", err)
	}
	if err := Update(func(cfg *Config) error { cfg.UILanguage = languageSpacedEnglish; return nil }); err != nil {
		t.Fatal(err)
	}
	values, err = readValues(path)
	if err != nil || values["ZEV_LANG"] != "en" {
		t.Fatalf("updated language=%q, error=%v", values["ZEV_LANG"], err)
	}
}
