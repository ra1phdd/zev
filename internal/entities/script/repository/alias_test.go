package repository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestTrustedParentAlias(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	alias := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(base, alias); err != nil {
		t.Fatal(err)
	}
	r := New(filepath.Join(alias, ".zev"))
	if err := r.SaveAll(map[string]domain.Script{safeName: {Name: safeName, ScriptFile: safeFile}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadAll(); err != nil {
		t.Fatal(err)
	}
}

func TestTemporaryHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := New(filepath.Join(os.Getenv("HOME"), ".zev"))
	if err := r.SaveAll(map[string]domain.Script{}); err != nil {
		t.Fatal(err)
	}
}
