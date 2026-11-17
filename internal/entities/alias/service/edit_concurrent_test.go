package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/repository"
)

func TestEditDoesNotOverwriteConcurrentTemplate(t *testing.T) {
	dir := t.TempDir()
	repo := repository.New(dir)
	svc := New(repo)
	if err := svc.Add("deploy", "echo original"); err != nil {
		t.Fatal(err)
	}
	editorPath := filepath.Join(t.TempDir(), "competing-editor.sh")
	script := "#!/bin/sh\nprintf 'deploy: echo concurrent\\n' > \"$ALIAS_FIXTURE_PATH\"\nprintf 'echo edited\\n' > \"$1\"\n"
	if err := os.WriteFile(editorPath, []byte(script), 0o700); err != nil { //nolint:gosec // Isolated executable editor fixture.
		t.Fatal(err)
	}
	t.Setenv("SCRIPT_EDITOR", editorPath)
	t.Setenv("ALIAS_FIXTURE_PATH", filepath.Join(dir, "aliases"))
	if err := svc.Edit(t.Context(), "deploy"); !errors.Is(err, ErrAliasChanged) {
		t.Errorf("stale edit error=%v, want ErrAliasChanged", err)
	}
	got, err := svc.Resolve("deploy", nil)
	if err != nil || got != "echo concurrent" {
		t.Fatalf("newer alias lost: %q error=%v", got, err)
	}
}
