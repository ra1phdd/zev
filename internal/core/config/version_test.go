package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func useConfigFile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, configFilename)
}

//nolint:paralleltest // useConfigFile changes the process home directory.
func TestConfigRejectsUnsupportedVersionWithoutWriting(t *testing.T) {
	for _, prefix := range []string{"", "version: 0\n", "version: 2\n", "version: '1'\n"} {
		path := useConfigFile(t)
		source := prefix + unversionedYAML
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(); err == nil {
			t.Error("Load accepted unsupported version")
		}
		if _, err := LoadUILanguage(); err == nil {
			t.Error("language lookup accepted unsupported version")
		}
		if err := Save(DefaultConfig()); err == nil {
			t.Error("Save accepted unsupported version")
		}
		if err := Update(func(*Config) error { t.Fatal("Update called callback"); return nil }); err == nil {
			t.Error("Update accepted unsupported version")
		}
		if data, err := os.ReadFile(filepath.Clean(path)); err != nil || string(data) != source {
			t.Fatal("unsupported file changed")
		}
	}
}

//nolint:paralleltest // useConfigFile changes the process home directory.
func TestSaveWritesV1(t *testing.T) {
	path := useConfigFile(t)
	if err := Save(DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil || !bytes.HasPrefix(data, []byte("version: 1\n")) {
		t.Fatalf("missing integer version on save: %v", err)
	}
}
