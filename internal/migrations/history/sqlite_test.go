package history

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
	_ "modernc.org/sqlite"
)

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestMigrateCreatesFreshReleaseBaseline(t *testing.T) {
	t.Parallel()
	db := openMigrationTestDB(t)
	if err := migrateSQLite(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	assertMigrationVersion(t, db, 1)
	var dirty bool
	if err := db.QueryRowContext(t.Context(), `SELECT dirty FROM schema_migrations`).Scan(&dirty); err != nil || dirty {
		t.Fatalf("migration dirty=%t err=%v", dirty, err)
	}
	for _, table := range []string{"history_schema", "history_writes", "history_fts_surface", "history_fts_morphology", "history_fts_trigram"} {
		var count int
		if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_schema WHERE name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("baseline object %s count=%d err=%v", table, count, err)
		}
	}
}

func TestMigrateCurrentDatabaseIsIdempotent(t *testing.T) {
	t.Parallel()
	db := openMigrationTestDB(t)
	if err := migrateSQLite(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	assertMigrationVersion(t, db, 1)
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM schema_migrations`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("ledger rows=%d err=%v", rows, err)
	}
}

func TestValidateCurrentSchema(t *testing.T) {
	t.Parallel()
	db := openMigrationTestDB(t)
	if err := migrateSQLite(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := Validate(t.Context(), db); err != nil {
		t.Fatalf("Validate() rejected current schema: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE history_writes`); err != nil {
		t.Fatal(err)
	}
	if err := Validate(t.Context(), db); !errors.Is(err, service.ErrUnsupportedSchema) {
		t.Fatalf("Validate() error = %v, want unsupported schema", err)
	}
}

func TestMigrateRejectsDevelopmentDatabaseUnchanged(t *testing.T) {
	t.Parallel()
	db := openMigrationTestDB(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE conversations(id TEXT PRIMARY KEY); CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL); INSERT INTO schema_migrations VALUES(1,'2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	before := databaseBytes(t, db)
	if err := migrateSQLite(t.Context(), db); !errors.Is(err, service.ErrUnsupportedSchema) {
		t.Fatalf("development database error=%v, want unsupported schema", err)
	}
	if after := databaseBytes(t, db); string(after) != string(before) {
		t.Fatal("development database changed after rejection")
	}
}

func TestMigrateRejectsFutureDatabaseUnchanged(t *testing.T) {
	t.Parallel()
	db := openMigrationTestDB(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations(version INTEGER NOT NULL,dirty INTEGER NOT NULL); INSERT INTO schema_migrations VALUES(2,0)`); err != nil {
		t.Fatal(err)
	}
	before := databaseBytes(t, db)
	if err := migrateSQLite(t.Context(), db); !errors.Is(err, service.ErrUnsupportedSchema) {
		t.Fatalf("future database error=%v, want unsupported schema", err)
	}
	if after := databaseBytes(t, db); string(after) != string(before) {
		t.Fatal("future database changed after rejection")
	}
}

func assertMigrationVersion(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(t.Context(), `SELECT version FROM schema_migrations`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("migration version=%d,want %d", got, want)
	}
}

func databaseBytes(t *testing.T, db *sql.DB) []byte {
	t.Helper()
	var name, path string
	if err := db.QueryRowContext(t.Context(), `PRAGMA database_list`).Scan(new(int), &name, &path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // Test database path comes from the test fixture.
	if err != nil {
		t.Fatal(err)
	}
	return data
}
