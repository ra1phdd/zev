package repository

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const historyWindowsOS = "windows"

func TestOpenDoesNotImportLegacySource(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	source := filepath.Join(dir, "history")
	original := []byte("not parsed by repository.Open\n")
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, s) })
	actual, err := os.ReadFile(source) //nolint:gosec // The test path is created inside t.TempDir().
	if err != nil || string(actual) != string(original) {
		t.Fatalf("legacy source changed: %q %v", actual, err)
	}
	var count int
	if err := s.db.QueryRowContext(t.Context(), `SELECT count(*) FROM conversations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("repository.Open imported %d conversations", count)
	}
}

func TestOpenCreatesPrivateDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "zev", "history.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, s) })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != historyWindowsOS && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("database permissions: %o", info.Mode().Perm())
	}
	var version int
	if err := s.db.QueryRowContext(t.Context(), "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != 3 {
		t.Fatalf("migration version %d: %v", version, err)
	}
	var ledger int
	if err := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='history_writes'").Scan(&ledger); err != nil || ledger != 1 {
		t.Fatalf("creation ledger count %d: %v", ledger, err)
	}
}

func TestOpenRejectsPublicHistoryDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows history privacy uses DACLs rather than Unix modes")
	}
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // The test deliberately creates an insecure application directory.
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), filepath.Join(dir, "history.db")); err == nil {
		t.Fatal("accepted history directory with group/world permissions")
	}
}

func TestOpenSecuresExistingDatabaseSidecars(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.WriteFile(name, nil, 0o644); err != nil { //nolint:gosec // The test deliberately creates overly broad database permissions.
			t.Fatal(err)
		}
	}
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, s) })
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		info, statErr := os.Stat(name)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if runtime.GOOS != historyWindowsOS && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s permissions: %o", name, info.Mode().Perm())
		}
	}
}
