package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/repository"
	historymigration "github.com/ra1phdd/zev/internal/migrations/history"
)

func TestSQLiteStoreRejectsSymlinkDatabase(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dir, "victim.db")
	if err := historymigration.Migrate(t.Context(), victim, true); err != nil {
		t.Fatal(err)
	}
	store, err := repository.Open(t.Context(), victim)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
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
	if len(data) == 0 {
		t.Fatal("valid SQLite victim database is empty")
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
