//go:build darwin || linux

package history

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestImportLegacyTightensWideSourceBeforeMigration(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	source := filepath.Join(filepath.Dir(path), "history")
	original := append(legacyTestLine(t), legacyTestLine(t)...)
	if err := os.WriteFile(source, original, 0o644); err != nil { //nolint:gosec // The fixture deliberately starts with a legacy-wide mode.
		t.Fatal(err)
	}
	if err := os.Chmod(source, 0o644); err != nil { //nolint:gosec // The fixture deliberately starts with a legacy-wide mode.
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		t.Fatalf("legacy source remains: %v", err)
	}
	if archives, err := filepath.Glob(source + ".migrated.*.jsonl"); err != nil || len(archives) != 0 {
		t.Fatalf("legacy archives = %v, %v", archives, err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 2)
	importTestCount(t, s, `SELECT count(DISTINCT id) FROM conversations`, 2)
	importTestCount(t, s, `SELECT count(*) FROM migration_runs WHERE status='completed'`, 1)
}

func TestImportLegacyTightensGroupWritableSource(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	source := filepath.Join(filepath.Dir(path), "history")
	original := legacyTestLine(t)
	if err := os.WriteFile(source, original, 0o664); err != nil { //nolint:gosec // The fixture deliberately starts with a group-writable legacy mode.
		t.Fatal(err)
	}
	if err := os.Chmod(source, 0o664); err != nil { //nolint:gosec // The fixture deliberately starts with a group-writable legacy mode.
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		t.Fatalf("legacy source remains: %v", err)
	}
	if archives, err := filepath.Glob(source + ".migrated.*.jsonl"); err != nil || len(archives) != 0 {
		t.Fatalf("legacy archives = %v, %v", archives, err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 1)
}

func TestImportLegacyRejectsHardlinkedSource(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.WriteFile(source, legacyTestLine(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, filepath.Join(filepath.Dir(path), "history-copy")); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); err == nil || !strings.Contains(err.Error(), "hard link") {
		t.Fatalf("hard-linked source result = %v", err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 0)
}

func TestSecureLegacyFileReportsClosedDescriptorFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "history")
	if err := os.WriteFile(source, legacyTestLine(t), 0o644); err != nil { //nolint:gosec // The fixture deliberately starts with a legacy-wide mode.
		t.Fatal(err)
	}
	if err := os.Chmod(source, 0o644); err != nil { //nolint:gosec // The fixture deliberately starts with a legacy-wide mode.
		t.Fatal(err)
	}
	file, err := openLegacySource(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := secureLegacyFile(file); err == nil {
		t.Fatal("accepted closed descriptor")
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("fixture mode changed after failed repair: %o", info.Mode().Perm())
	}
}

func TestImportLegacyRejectsFIFOWithoutOpeningIt(t *testing.T) {
	t.Parallel()
	_, path := importTestStore(t)
	fifo := filepath.Join(filepath.Dir(path), "history")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	if err := Migrate(ctx, path, false); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("FIFO result = %v", err)
	}
}
