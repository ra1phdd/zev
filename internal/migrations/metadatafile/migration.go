// Package metadatafile runs versioned, in-place metadata upgrades without archives.
package metadatafile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ra1phdd/zev/internal/migrations/filetxn"
)

// Apply validates a metadata document and replaces it only if upgraded. The
// caller holds the writer lock, including during validation of referenced files.
func (s Schema) Apply(ctx context.Context, dir string, validate func(data []byte, publishing bool) error) error {
	journal := filepath.Join(dir, "."+s.Entity+"-migration")
	if err := checkJournal(journal); err != nil {
		return err
	}
	path := filepath.Join(dir, s.Entity)
	source, err := filetxn.ReadSnapshot(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", s.Entity, err)
	}
	data, changed, err := s.Upgrade(ctx, source.Bytes())
	if err != nil {
		return err
	}
	if err := validate(data, changed); err != nil {
		return fmt.Errorf("validate %s: %w", s.Entity, err)
	}
	if !changed {
		return nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect metadata source: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("metadata source must be private")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("publish %s: %w", s.Entity, err)
	}
	if err := source.Replace(data); err != nil {
		return fmt.Errorf("replace %s: %w", s.Entity, err)
	}
	return nil
}

// Development journals were never released. Do not interpret their contents
// or sweep a markerless directory, which can contain user-owned backups.
func checkJournal(dir string) error {
	info, err := os.Lstat(filepath.Clean(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect migration directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("unexpected migration directory type")
	}
	_, err = os.Lstat(filepath.Join(dir, "marker"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect migration marker: %w", err)
	}
	return errors.New("unresolved metadata migration marker; inspect manually before retrying")
}
