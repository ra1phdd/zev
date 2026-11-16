package repository

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestMetadataDirectorySyncFailureIsPostCommit(t *testing.T) {
	t.Parallel()
	r := New(testDir(t))
	if err := r.Create(t.Context(), domain.Script{Name: safeName, ScriptFile: safeFile}, []byte("original")); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected metadata directory sync failure")
	r.syncMetadataDirectory = func(*os.Root) error { return failure }
	if err := r.Create(t.Context(), domain.Script{Name: "new", ScriptFile: "new.sh"}, []byte("new")); !errors.Is(err, ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("Create error = %v, want committed sync failure", err)
	}
	scripts, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scripts["new"]; !ok {
		t.Fatalf("metadata replacement was rolled back: %#v", scripts)
	}
	data, err := os.ReadFile(filepath.Clean(filepath.Join(r.scriptsDir, "new.sh")))
	if err != nil || string(data) != "new" {
		t.Fatalf("created script was rolled back: %q, error = %v", data, err)
	}
	if err := r.Remove("new"); !errors.Is(err, ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("Remove error = %v, want committed sync failure", err)
	}
	entries, err := os.ReadDir(r.scriptsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != safeFile {
		t.Fatalf("committed removal left rollback files: %v", entries)
	}
}
