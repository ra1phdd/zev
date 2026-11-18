package history

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/repository"
)

func TestMigrateCreatePolicyOnEmptyStore(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "missing")
	path := filepath.Join(dir, "history.db")
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("offline migration created parent: %v", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("offline migration changed empty storage: %v %v", entries, err)
	}
	if err := Migrate(t.Context(), path, true); err != nil {
		t.Fatal(err)
	}
	store, err := repository.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, store) })
}

func TestMigrateExistingSQLiteWithoutLegacyIsIdempotent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "zev", "history.db")
	db, err := repository.OpenDatabase(t.Context(), path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(t.Context(), db); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatal(errors.Join(err, closeErr))
		}
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := Migrate(t.Context(), path, false); err != nil {
			t.Fatal(err)
		}
	}
	db, err = repository.OpenDatabase(t.Context(), path, false)
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

func TestMigrateRecoversVerifiedLedgerAfterSourceRemoval(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	run := prepareVerifiedImport(t, s, path)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := s.db.QueryRowContext(t.Context(), `SELECT status FROM migration_runs WHERE id=?`, run.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("ledger status=%s", status)
	}
}
