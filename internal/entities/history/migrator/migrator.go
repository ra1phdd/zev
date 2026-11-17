package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/entities/history/repository"
)

type migrator struct{ db *sql.DB }

// Migrate imports the active legacy JSONL file, archives it exclusively, and
// completes or recovers the durable migration ledger. It is idempotent.
func Migrate(ctx context.Context, dbPath string) error {
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
	if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
		if _, dbErr := os.Lstat(abs); errors.Is(dbErr, os.ErrNotExist) {
			return nil
		} else if dbErr != nil {
			return fmt.Errorf("inspect history database: %w", dbErr)
		}
	} else if err != nil {
		return fmt.Errorf("inspect legacy history: %w", err)
	}
	if err := filestore.WithLockContext(ctx, source, func() (err error) {
		_, sourceErr := os.Lstat(source)
		create := sourceErr == nil
		if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
			return fmt.Errorf("inspect legacy history: %w", sourceErr)
		}
		db, err := repository.OpenDatabase(ctx, abs, create)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && !create {
				return nil
			}
			return fmt.Errorf("open history database: %w", err)
		}
		defer func() { err = errors.Join(err, db.Close()) }()
		return (&migrator{db: db}).importLegacyLocked(ctx, source)
	}); err != nil {
		return fmt.Errorf("lock legacy migration: %w", err)
	}
	return nil
}

func rollbackMigration(err *error, tx *sql.Tx) {
	if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		*err = errors.Join(*err, fmt.Errorf("rollback transaction: %w", rollbackErr))
	}
}
