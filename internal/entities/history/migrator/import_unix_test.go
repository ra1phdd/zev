//go:build darwin || linux

package migrator

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
	if err := s.importLegacy(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(source + ".migrated.*.jsonl")
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives = %v, %v", archives, err)
	}
	archiveInfo, err := os.Stat(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	if archiveInfo.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %o, want 600", archiveInfo.Mode().Perm())
	}
	archived, err := os.ReadFile(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(archived) != string(original) {
		t.Fatalf("archive changed: got %q, want %q", archived, original)
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
	if err := s.importLegacy(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(source + ".migrated.*.jsonl")
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives = %v, %v", archives, err)
	}
	info, err := os.Stat(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %o, want 600", info.Mode().Perm())
	}
	archived, err := os.ReadFile(archives[0])
	if err != nil || string(archived) != string(original) {
		t.Fatalf("archive changed: %q %v", archived, err)
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
	if err := s.importLegacy(t.Context(), path); err == nil || !strings.Contains(err.Error(), "hard link") {
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
	s, path := importTestStore(t)
	fifo := filepath.Join(filepath.Dir(path), "history")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	if err := s.importLegacy(ctx, path); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("FIFO result = %v", err)
	}
}

func TestImportLegacyRejectsArchiveHardlink(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	run := prepareVerifiedImport(t, s, path)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := renameExclusive(source, run.Metadata.ArchivedPath); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(path), "target")
	if err := os.WriteFile(target, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(run.Metadata.ArchivedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, run.Metadata.ArchivedPath); err != nil {
		t.Fatal(err)
	}
	if err := s.importLegacy(t.Context(), path); err == nil || !strings.Contains(err.Error(), "hard link") {
		t.Fatalf("hard-linked archive result = %v", err)
	}
	data, err := os.ReadFile(target) //nolint:gosec // The test path is created inside t.TempDir().
	if err != nil || string(data) != "unrelated" {
		t.Fatalf("target changed: %q %v", data, err)
	}
}

func TestImportLegacyRejectsArchiveSymlinkWithoutChmodTarget(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.WriteFile(source, legacyTestLine(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.importLegacy(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	var run legacyRun
	if _, err := s.db.ExecContext(t.Context(), `UPDATE migration_runs SET status='verified',completed_at=NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.readUnfinishedLegacyRun(t.Context(), &run); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(path), "target")
	if err := os.WriteFile(target, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil { //nolint:gosec // The test deliberately creates a too-wide archive target.
		t.Fatal(err)
	}
	if err := os.Remove(run.Metadata.ArchivedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, run.Metadata.ArchivedPath); err != nil {
		t.Fatal(err)
	}
	if err := s.importLegacy(t.Context(), path); err == nil {
		t.Fatal("accepted archive symlink")
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("target permissions changed: %v, %v", info, err)
	}
}
