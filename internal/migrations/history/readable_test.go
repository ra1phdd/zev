package history

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestCheckReadableDoesNotCreateHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.db")
	if err := CheckReadable(t.Context(), path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CheckReadable error = %v, want os.ErrNotExist", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("history database stat error = %v, want os.ErrNotExist", err)
	}
}

func TestCheckReadableAcceptsCurrentHistory(t *testing.T) {
	t.Parallel()
	db := openMigrationTestDB(t)
	if err := migrateSQLite(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	path := migrationDatabasePath(t, db)
	if err := CheckReadable(t.Context(), path); err != nil {
		t.Fatalf("CheckReadable error = %v", err)
	}
}

func TestCheckReadableRejectsUnsupportedSchema(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		corrupt string
	}{
		{name: "missing current table", corrupt: `DROP TABLE history_writes`},
		{name: "future", corrupt: `UPDATE schema_migrations SET version=2`},
		{name: "dirty", corrupt: `UPDATE schema_migrations SET dirty=1`},
		{name: "wrong identity", corrupt: `DELETE FROM history_schema WHERE id=1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := openMigrationTestDB(t)
			if err := migrateSQLite(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), tc.corrupt); err != nil {
				t.Fatal(err)
			}
			path := migrationDatabasePath(t, db)
			if err := CheckReadable(t.Context(), path); !errors.Is(err, service.ErrUnsupportedSchema) {
				t.Fatalf("CheckReadable error = %v, want unsupported schema", err)
			}
		})
	}
}

func migrationDatabasePath(t *testing.T, db *sql.DB) string {
	t.Helper()
	var name, path string
	if err := db.QueryRowContext(t.Context(), `PRAGMA database_list`).Scan(new(int), &name, &path); err != nil {
		t.Fatal(err)
	}
	return path
}
