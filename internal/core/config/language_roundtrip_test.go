package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUILanguageLoadRoundTripAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	t.Setenv("ZEV_LANG", "")
	if err := os.Unsetenv("ZEV_LANG"); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.UILanguage = "ru"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil || loaded.UILanguage != "ru" {
		t.Fatalf("file round trip: error=%v", err)
	}
	t.Setenv("ZEV_LANG", " EN ")
	loaded, err = Load()
	if err != nil || loaded.UILanguage != "en" {
		t.Fatalf("environment override: error=%v", err)
	}
	if err := Update(func(cfg *Config) error { cfg.ScriptEditor = "test-editor"; return nil }); err != nil {
		t.Fatal(err)
	}
	values, err := readValues(path)
	if err != nil || values["ZEV_LANG"] != "ru" {
		t.Fatalf("ambient language leaked into Update: error=%v", err)
	}
	t.Setenv("ZEV_LANG", "synthetic-invalid-language")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted invalid environment language")
	}
	if _, err := LoadUILanguage(); err == nil {
		t.Fatal("LoadUILanguage accepted invalid environment language")
	}
}
