package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
	"modernc.org/sqlite"
)

func TestClassifyStorageErrorSanitizesFailure(t *testing.T) {
	t.Parallel()

	cause := &os.PathError{Op: "open", Path: "private-payload.db", Err: os.ErrPermission}
	input := fmt.Errorf("SELECT private_payload: %w", cause)
	got := classifyStorageError(input)
	if got == nil || got.Error() != "history storage failure" {
		t.Fatalf("error = %v, want sanitized storage failure", got)
	}
	if !errors.Is(got, service.ErrStorageFailure) || !errors.Is(got, input) || !errors.Is(got, os.ErrPermission) {
		t.Fatalf("error did not preserve classification and causes: %v", got)
	}
	var pathErr *os.PathError
	if !errors.As(got, &pathErr) || pathErr != cause {
		t.Fatal("error did not preserve PathError identity")
	}
	if strings.Contains(got.Error(), "private_payload") || strings.Contains(got.Error(), "private-payload.db") {
		t.Fatalf("error leaked private details: %q", got)
	}
}

func TestClassifyStorageErrorTypedMappings(t *testing.T) {
	t.Parallel()

	unique, foreignKey := sqliteConstraintErrors(t)
	busy := sqliteBusyError(t)
	tests := []struct {
		name  string
		input error
		want  error
		text  string
	}{
		{name: "cancelled", input: fmt.Errorf("operation payload: %w", context.Canceled), want: service.ErrCancelled, text: "history operation cancelled"},
		{name: "deadline", input: context.DeadlineExceeded, want: service.ErrCancelled, text: "history operation cancelled"},
		{name: "not found", input: fmt.Errorf("query payload: %w", sql.ErrNoRows), want: service.ErrNotFound, text: "history not found"},
		{name: "busy", input: busy, want: service.ErrBusy, text: "history storage busy"},
		{name: "unique constraint", input: unique, want: service.ErrConflict, text: "history conflict"},
		{name: "foreign key constraint", input: foreignKey, want: service.ErrInvalid, text: "history invalid input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := classifyStorageError(test.input)
			if got == nil || got.Error() != test.text {
				t.Fatalf("error = %v, want %q", got, test.text)
			}
			if !errors.Is(got, test.want) || !errors.Is(got, test.input) {
				t.Fatalf("error = %v, want %v and original cause", got, test.want)
			}
			if strings.Contains(got.Error(), "payload") || strings.Contains(got.Error(), "items") {
				t.Fatalf("error leaked cause details: %q", got)
			}
		})
	}
}

func TestClassifyStorageErrorPreservesExistingSentinels(t *testing.T) {
	t.Parallel()

	for _, sentinel := range []error{service.ErrNotFound, service.ErrConflict, service.ErrInvalid, service.ErrUnsupportedSchema, service.ErrBusy, service.ErrCancelled, service.ErrStorageFailure, service.ErrEmbeddingUnavailable} {
		if got := classifyStorageError(sentinel); !errors.Is(got, sentinel) {
			t.Fatalf("classifyStorageError(%v) = %v, want same classification", sentinel, got)
		}
	}
	if classifyStorageError(nil) != nil {
		t.Fatal("classifyStorageError(nil) must return nil")
	}
}

func sqliteConstraintErrors(t *testing.T) (unique, foreignKey error) {
	t.Helper()
	db := openSQLiteErrorTestDB(t)

	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys=ON;
CREATE TABLE parents(id INTEGER PRIMARY KEY);
CREATE TABLE children(parent_id INTEGER REFERENCES parents(id));
CREATE TABLE items(value TEXT UNIQUE);
INSERT INTO items(value) VALUES ('one');`); err != nil {
		t.Fatal(err)
	}
	if _, unique = db.ExecContext(t.Context(), `INSERT INTO items(value) VALUES ('one')`); unique == nil {
		t.Fatal("duplicate insert unexpectedly succeeded")
	}
	if _, foreignKey = db.ExecContext(t.Context(), `INSERT INTO children(parent_id) VALUES (42)`); foreignKey == nil {
		t.Fatal("foreign-key insert unexpectedly succeeded")
	}
	return fmt.Errorf("unique fixture: %w", unique), fmt.Errorf("foreign key fixture: %w", foreignKey)
}

func sqliteBusyError(t *testing.T) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "busy.db")
	db1 := openSQLiteErrorTestDBAt(t, path)
	db2 := openSQLiteErrorTestDBAt(t, path)
	t.Cleanup(func() {
		if err := errors.Join(db1.Close(), db2.Close()); err != nil {
			t.Error(err)
		}
	})
	if _, err := db1.ExecContext(t.Context(), `CREATE TABLE locks(value TEXT)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db1.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	})
	if _, err = tx.ExecContext(t.Context(), `INSERT INTO locks(value) VALUES ('held')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db2.ExecContext(t.Context(), `INSERT INTO locks(value) VALUES ('blocked')`); err == nil {
		t.Fatal("blocked insert unexpectedly succeeded")
	} else {
		if sqliteErr, ok := errors.AsType[*sqlite.Error](err); !ok || sqliteErr == nil {
			t.Fatalf("busy error = %v, want modernc sqlite error", err)
		}
		return fmt.Errorf("busy fixture: %w", err)
	}
	return nil
}

func openSQLiteErrorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return openSQLiteErrorTestDBAt(t, filepath.Join(t.TempDir(), "errors.db"))
}

func openSQLiteErrorTestDBAt(t *testing.T, path string) *sql.DB {
	t.Helper()
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(0)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	db.SetMaxOpenConns(1)
	if err = db.PingContext(t.Context()); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal(err)
	}
	return db
}
