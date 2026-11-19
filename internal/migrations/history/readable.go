package history

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
	// Register SQLite so database/sql can open the read-only history connection.
	_ "modernc.org/sqlite"
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
	query := url.Values{}
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "busy_timeout(1000)")
	db, err := sql.Open("sqlite", historyReadOnlyURL(abs, query))
	if err != nil {
		return fmt.Errorf("open read-only history: %w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err := Validate(ctx, db); err != nil {
		return fmt.Errorf("check history schema: %w", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM (SELECT id FROM conversations LIMIT 1)").Scan(&count); err != nil {
		return fmt.Errorf("read history records: %w", err)
	}
	return nil
}

func historyReadOnlyURL(abs string, query url.Values) string {
	path := filepath.ToSlash(abs)
	u := &url.URL{Scheme: "file"}
	switch {
	case len(path) >= 2 && path[1] == ':':
		u.Path = "/" + path
	case strings.HasPrefix(path, "//"):
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
	u.RawQuery = query.Encode()
	return u.String()
}
