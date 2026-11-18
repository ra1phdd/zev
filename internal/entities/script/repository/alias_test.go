package repository

import (
	"io"
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
	script := domain.Script{Name: safeName, ScriptFile: safeFile}
	if err := r.Create(t.Context(), script, []byte("body")); err != nil {
		t.Fatal(err)
	}
	scripts, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 1 || scripts[safeName].Name != script.Name || scripts[safeName].ScriptFile != script.ScriptFile || scripts[safeName].Description != script.Description || scripts[safeName].RecordID == "" {
		t.Fatalf("metadata = %+v, want %v with generated record ID", scripts, script)
	}
	file, err := r.OpenScriptPinned(t.Context(), scripts[safeName], nil)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(file)
	if err := file.Close(); err != nil {
		t.Error(err)
	}
	if readErr != nil || string(data) != scriptBody {
		t.Errorf("script = %q, %v", data, readErr)
	}
}

func TestTemporaryHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := New(filepath.Join(os.Getenv("HOME"), ".zev"))
	scripts, err := r.LoadAll()
	if err != nil || len(scripts) != 0 {
		t.Fatalf("new store metadata = %+v, %v", scripts, err)
	}
	script := domain.Script{Name: safeName, ScriptFile: safeFile}
	if err := r.Create(t.Context(), script, []byte("body")); err != nil {
		t.Fatal(err)
	}
	scripts, err = r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 1 || scripts[safeName].Name != script.Name || scripts[safeName].ScriptFile != script.ScriptFile || scripts[safeName].Description != script.Description || scripts[safeName].RecordID == "" {
		t.Fatalf("metadata = %+v, want %v with generated record ID", scripts, script)
	}
}
