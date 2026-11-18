package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

// CheckReadable checks an existing history database without creating or migrating
// it. It returns os.ErrNotExist through errors.Is for an uninitialized database,
// and service.ErrUnsupportedSchema for an incompatible version. No history
// content is returned. The caller must provide a bounded context.
func CheckReadable(ctx context.Context, path string) (err error) {
	if err := filestore.ValidateDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("validate history directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect history: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("history is not a regular file")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve history: %w", err)
	}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "busy_timeout(1000)")
	db, err := sql.Open("sqlite", historySQLiteURL(abs, q))
	if err != nil {
		return fmt.Errorf("open read-only history: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	var version int
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("read history version: %w", err)
	}
	if version != 3 {
		return fmt.Errorf("%w: history requires migration or a compatible zev version", service.ErrUnsupportedSchema)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM (SELECT id FROM conversations LIMIT 1)").Scan(&count); err != nil {
		return fmt.Errorf("read history records: %w", err)
	}
	return nil
}
