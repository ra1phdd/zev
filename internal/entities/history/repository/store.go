// Package repository stores history in SQLite.
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

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	_ "modernc.org/sqlite" // Register the SQLite database/sql driver.
)

const historySQLiteURLScheme = "file"

// Store persists history in a SQLite database.
type Store struct{ db *sql.DB }

// Open opens an existing private SQLite history database. The database must
// already have the released schema; the history migrator must run before this
// method. Open never bootstraps, migrates, or validates the schema.
// The caller owns the returned store and must close it.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := OpenDatabase(ctx, path, false)
	if err != nil {
		return nil, fmt.Errorf("initialize history: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return nil, errors.Join(fmt.Errorf("enable WAL: %w", err), db.Close())
	}
	return &Store{db: db}, nil
}

// OpenDatabase opens a raw SQLite connection for an explicit bootstrap or
// maintenance boundary. It does not validate or migrate the schema.
// When create is false, the database must already exist and no filesystem data
// is created. The returned connection is owned by the caller.
func OpenDatabase(ctx context.Context, path string, create bool) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty database path", service.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve history path: %w", err)
	}
	dir := filepath.Dir(abs)
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, fmt.Errorf("validate history directory: %w", err)
	}
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create history directory: %w", err)
		}
		if err := secureHistoryPath(dir, true); err != nil {
			return nil, fmt.Errorf("secure history directory: %w", err)
		}
	} else if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("inspect history database: %w", err)
	}
	db, err := openDatabase(ctx, abs, create)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func secureHistorySidecars(abs string) error {
	for _, sidecar := range []string{abs + "-wal", abs + "-shm"} {
		info, err := os.Lstat(sidecar)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect database sidecar: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: database sidecar is not a regular file", service.ErrInvalid)
		}
		if err := secureHistoryPath(sidecar, false); err != nil {
			return fmt.Errorf("secure database sidecar: %w", err)
		}
	}
	return nil
}

func historySQLiteURL(abs string, query url.Values) string {
	path := filepath.ToSlash(abs)
	u := &url.URL{Scheme: historySQLiteURLScheme}
	switch {
	case len(path) >= 2 && path[1] == ':':
		// SQLite treats file://C:/... as a URI with authority C:. A leading
		// slash makes the Windows drive path a local file URI.
		u.Path = "/" + path
	case strings.HasPrefix(path, "//"):
		// Preserve UNC paths as file://server/share/... URIs.
		rest := strings.TrimPrefix(path, "//")
		host, share, found := strings.Cut(rest, "/")
		if found {
			u.Host = host
			u.Path = "/" + share
		} else {
			u.Host = rest
		}
	default:
		u.Path = path
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func openDatabase(_ context.Context, abs string, create bool) (*sql.DB, error) {
	if err := secureHistorySidecars(abs); err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil && (!create || !os.IsNotExist(err)) {
		return nil, fmt.Errorf("inspect history database: %w", err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: database is not a regular file", service.ErrInvalid)
	}
	if err == nil {
		if err := secureHistoryPath(abs, false); err != nil {
			return nil, fmt.Errorf("secure history database: %w", err)
		}
	}
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	f, err := os.OpenFile(abs, flags, 0o600) //nolint:gosec // The caller-selected database leaf and trusted application directory were validated.
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	if err = secureHistoryPath(abs, false); err != nil {
		return nil, errors.Join(fmt.Errorf("secure history database: %w", err), f.Close())
	}
	if err = f.Close(); err != nil {
		return nil, fmt.Errorf("close history database: %w", err)
	}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Set("_txlock", "immediate")
	if !create {
		q.Set("mode", "rw")
	}
	db, err := sql.Open("sqlite", historySQLiteURL(abs, q))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close history database: %w", err)
	}
	return nil
}
