package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/repository"
)

func TestSQLiteStoreRejectsSymlinkDatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.db")
	if err := os.WriteFile(victim, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	if err := os.Symlink(victim, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := repository.Open(t.Context(), path); err == nil {
		t.Fatal("SQLite store accepted a symlink database path")
	}
	data, err := os.ReadFile(filepath.Clean(victim))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "not a database" {
		t.Fatalf("symlink target changed: %q", data)
	}
}

func TestSQLiteStoreRejectsSymlinkDirectory(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	target := filepath.Join(home, "external")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".zev")
	if err := os.Symlink(target, dir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := repository.Open(t.Context(), filepath.Join(dir, "history.db")); err == nil {
		t.Fatal("SQLite store accepted a symlink history directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("external directory modified: %v", entries)
	}
}
