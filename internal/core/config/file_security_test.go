package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:paralleltest // Mutates the package-global test config path.
func TestConfigRejectsGroupOrWorldPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	if err := os.WriteFile(path, []byte(minimalYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("private config rejected: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil { //nolint:gosec // Test fixture intentionally exercises unsafe permissions.
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "group or others") {
		t.Fatalf("unsafe config accepted: %v", err)
	}
}
