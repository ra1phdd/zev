package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	current "github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/migrations/filetxn"
)

// Migrate upgrades the v1.0.0 dotenv file in dir to version 1 YAML. It holds
// the source lock before taking the current writer's lock, including
// during recovery. A current file is validated without rewriting it.
func Migrate(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migrate configuration: %w", err)
	}
	if !filepath.IsAbs(dir) {
		return errors.New("expected absolute configuration directory")
	}
	if err := filestore.ValidateDirectory(dir); err != nil {
		return fmt.Errorf("validate migration directory: %w", err)
	}
	source, dest := filepath.Join(dir, "config"), filepath.Join(dir, "config.yaml")
	// Do not create a directory or lock files for an absent store.
	found := false
	for _, path := range []string{source, dest, dest + ".migration"} {
		if _, err := os.Lstat(filepath.Clean(path)); err == nil {
			found = true
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect migration inputs: %w", err)
		}
	}
	if !found {
		return nil
	}
	if err := filestore.WithLockContext(ctx, source, func() error {
		return filestore.WithLockContext(ctx, dest, func() error {
			// Recovery can retire a journal and remove its source. Validate any
			// already published destination before authorizing that cleanup.
			data, err := filetxn.Read(dest)
			switch {
			case err == nil:
				if _, err := current.Parse(data); err != nil {
					return fmt.Errorf("validate existing configuration: %w", err)
				}
			case !errors.Is(err, os.ErrNotExist):
				return fmt.Errorf("read existing configuration: %w", err)
			}
			if err := filetxn.Migrate(source, dest, dotenvToV1); err != nil {
				return fmt.Errorf("publish migrated configuration: %w", err)
			}
			return nil
		})
	}); err != nil {
		return fmt.Errorf("migrate configuration: %w", err)
	}
	return nil
}
