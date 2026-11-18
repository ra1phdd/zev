package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

func closeImportTest(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Error(err)
	}
}

func openTestStore(ctx context.Context, t *testing.T, path string) (*Store, error) {
	t.Helper()
	// Repository tests need the current schema, not the migration path that
	// imports this package. Reuse its SQL and seed an already-applied ledger.
	baseline, err := os.ReadFile("../../../migrations/history/1_history.up.sql")
	if err != nil {
		return nil, fmt.Errorf("read history test baseline: %w", err)
	}
	db, err := OpenDatabase(ctx, path, true)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, string(baseline)); err != nil {
		return nil, errors.Join(fmt.Errorf("apply history test baseline: %w", err), db.Close())
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER NOT NULL, dirty INTEGER NOT NULL);
		CREATE UNIQUE INDEX version_unique ON schema_migrations(version);
		INSERT INTO schema_migrations VALUES(1, 0);`); err != nil {
		return nil, errors.Join(fmt.Errorf("seed history test ledger: %w", err), db.Close())
	}
	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("close test database: %w", err)
	}
	return Open(ctx, path)
}
