package migrator

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/repository"
)

func TestMigrateDoesNotCreateDatabaseWithoutLegacySource(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing", "history.db")
	if err := Migrate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Migrate created parent: %v", err)
	}
}

func TestMigrateExistingSQLiteWithoutLegacyIsIdempotent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "zev", "history.db")
	store, err := repository.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := Migrate(t.Context(), path); err != nil {
			t.Fatal(err)
		}
	}
	db, err := repository.OpenDatabase(t.Context(), path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, table := range []string{"migration_runs", "conversations"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("unexpected rows in %s: %d", table, count)
		}
	}
}

func TestMigrateRecoversArchivedLedgerWithoutSource(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	run := prepareVerifiedImport(t, s, path)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := renameExclusive(source, run.Metadata.ArchivedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(run.Metadata.ArchivedPath, 0o644); err != nil { //nolint:gosec // The fixture deliberately starts with a legacy-wide archive mode.
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := s.db.QueryRowContext(t.Context(), `SELECT status FROM migration_runs WHERE id=?`, run.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("ledger status=%s", status)
	}
	info, err := os.Stat(run.Metadata.ArchivedPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %o, want 600", info.Mode().Perm())
	}
}
