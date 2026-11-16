package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

//nolint:paralleltest // Mutates the global config path.
func TestConfigRejectsUnsafeRead(t *testing.T) {
	for _, kind := range []string{"symlink", "dangling symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			victim := filepath.Join(dir, "victim")
			if err := os.WriteFile(victim, []byte("OPENAI_MODEL=external\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "config")
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(victim, path)
			case "dangling symlink":
				err = os.Symlink(filepath.Join(dir, "missing"), path)
			case "directory":
				err = os.Mkdir(path, 0o700)
			}
			if err != nil {
				t.Skipf("cannot create %s: %v", kind, err)
			}

			previous := getConfigPath
			getConfigPath = func() string { return path }
			t.Cleanup(func() { getConfigPath = previous })
			if _, err := Load(); err == nil {
				t.Error("Load accepted unsafe file")
			}
			called := false
			if err := Update(func(*Config) error { called = true; return nil }); err == nil || called {
				t.Errorf("Update accepted unsafe file: error = %v, callback = %v", err, called)
			}

			data, err := os.ReadFile(filepath.Clean(victim))
			if err != nil || string(data) != "OPENAI_MODEL=external\n" {
				t.Fatalf("victim changed: %q, %v", data, err)
			}
			info, err := os.Lstat(path)
			if err != nil || info.Mode().IsRegular() {
				t.Fatalf("unsafe leaf replaced: %v, %v", info, err)
			}
		})
	}
}

//nolint:paralleltest // Mutates the global config path.
func TestConfigRejectsUnsafeDirectory(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(fmt.Sprint("dangling=", dangling), func(t *testing.T) {
			home := t.TempDir()
			target := filepath.Join(home, "external")
			if !dangling {
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(home, ".zev")
			if err := os.Symlink(target, dir); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			path := filepath.Join(dir, "config")
			previous := getConfigPath
			getConfigPath = func() string { return path }
			t.Cleanup(func() { getConfigPath = previous })
			if _, err := Load(); err == nil {
				t.Error("Load accepted unsafe directory")
			}
			if err := Save(DefaultConfig()); err == nil {
				t.Error("Save accepted unsafe directory")
			}
			called := false
			if err := Update(func(*Config) error { called = true; return nil }); err == nil || called {
				t.Errorf("Update accepted unsafe directory: %v, callback = %v", err, called)
			}
			if dangling {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("target created: %v", err)
				}
			} else {
				entries, err := os.ReadDir(target)
				if err != nil || len(entries) != 0 {
					t.Fatalf("external directory modified: %v, %v", entries, err)
				}
			}
		})
	}
}
