package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
)

func TestEditUpdatesAliasThroughConfiguredEditor(t *testing.T) {
	dir := t.TempDir()
	repo := repository.New(dir)
	if err := repo.Update(func(aliases map[string]domain.Alias) error {
		aliases["deploy"] = domain.Alias{Name: "deploy", Template: "echo before"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	editorPath := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf 'echo after' > \"$1\"\n"), 0o700); err != nil { //nolint:gosec // Test fixture is isolated and intentionally executable.
		t.Fatal(err)
	}
	t.Setenv("SCRIPT_EDITOR", editorPath)
	if err := New(repo).Edit(t.Context(), "deploy"); err != nil {
		t.Fatal(err)
	}
	got, err := New(repo).Resolve("deploy", nil)
	if err != nil || got != "echo after" {
		t.Fatalf("resolved alias = %q, err=%v", got, err)
	}
}
