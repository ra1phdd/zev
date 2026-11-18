package repository

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestHistoryRejectsInvalidLedger(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, corrupt string }{
		{"missing current table", `DROP TABLE history_writes`},
		{"future", `UPDATE schema_migrations SET version=2`},
		{"dirty", `UPDATE schema_migrations SET dirty=1`},
		{"wrong identity", `DELETE FROM history_schema WHERE id=1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "zev", "history.db")
			store, err := openTestStore(t.Context(), t, path)
			if err != nil {
				t.Fatal(err)
			}
			db := store.db
			t.Cleanup(func() { closeImportTest(t, store) })
			if _, err := db.ExecContext(t.Context(), tc.corrupt); err != nil {
				t.Fatal(err)
			}
			if err := CheckReadable(t.Context(), path); !errors.Is(err, service.ErrUnsupportedSchema) {
				t.Errorf("diagnostic: got %v, want unsupported schema", err)
			}
			reopened, err := Open(t.Context(), path)
			if reopened != nil {
				closeImportTest(t, reopened)
			}
			if !errors.Is(err, service.ErrUnsupportedSchema) {
				t.Errorf("migration: got %v, want unsupported schema", err)
			}
		})
	}
}
