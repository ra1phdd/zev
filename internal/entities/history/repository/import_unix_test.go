//go:build darwin || linux

package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

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
