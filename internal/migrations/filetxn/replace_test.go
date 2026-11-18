package filetxn

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceDoesNotArchiveOrOverwriteConflicts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata")
	fixture(t, path, "stale")
	stale, err := ReadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture(t, path, "original")
	if err := stale.Replace([]byte("converted")); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale source: %v", err)
	}
	checkContent(t, path, "original")
	source, err := ReadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Replace([]byte("converted")); err != nil {
		t.Fatal(err)
	}
	checkContent(t, path, "converted")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "metadata" {
		t.Fatalf("unexpected publication artifacts: %v, %v", entries, err)
	}
}
