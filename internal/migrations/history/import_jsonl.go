// Package history initializes SQLite history and imports released JSONL history.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/entities/history/repository"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

type migrator struct{ db *sql.DB }

// Migrate initializes the SQLite schema, imports any legacy JSONL, and removes
// the source after verification. Without createIfMissing, an absent store is
// left untouched. A legacy source always warrants creating its destination.
func Migrate(ctx context.Context, dbPath string, createIfMissing bool) error {
	if dbPath == "" {
		return fmt.Errorf("%w: empty database path", service.ErrInvalid)
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("resolve history path: %w", err)
	}
	dir := filepath.Dir(abs)
	if err := filestore.ValidateDirectory(dir); err != nil {
		return fmt.Errorf("validate migration directory: %w", err)
	}
	source := filepath.Join(dir, "history")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migrate history: %w", err)
	}
	if !createIfMissing {
		present, err := historyExists(source, abs)
		if err != nil || !present {
			return err
		}
	}
	if err := filestore.WithLockContext(ctx, source, func() error {
		return migrateLocked(ctx, abs, source, createIfMissing)
	}); err != nil {
		return fmt.Errorf("migrate history under lock: %w", err)
	}
	return nil
}

func migrateLocked(ctx context.Context, dbPath, source string, createIfMissing bool) (err error) {
	_, sourceErr := os.Lstat(source)
	if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy history: %w", sourceErr)
	}
	create := createIfMissing || sourceErr == nil
	db, err := repository.OpenDatabase(ctx, dbPath, create)
	if errors.Is(err, os.ErrNotExist) && !create {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open history database: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err := migrateSQLite(ctx, db); err != nil {
		return fmt.Errorf("initialize release history schema: %w", err)
	}
	return (&migrator{db: db}).importLegacyLocked(ctx, source)
}

func historyExists(source, dbPath string) (bool, error) {
	for _, path := range []string{source, dbPath} {
		_, err := os.Lstat(path)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect history migration input: %w", err)
		}
	}
	return false, nil
}

func rollbackMigration(err *error, tx *sql.Tx) {
	if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		*err = errors.Join(*err, fmt.Errorf("rollback transaction: %w", rollbackErr))
	}
}
