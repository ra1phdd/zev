package config

import (
	"bytes"
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
			return migrateLocked(source, dest)
		})
	}); err != nil {
		return fmt.Errorf("migrate configuration: %w", err)
	}
	return nil
}

func migrateLocked(source, dest string) error {
	if _, err := os.Lstat(source); err == nil {
		// The file transaction must classify an existing destination before
		// normalization can rewrite it.
		if err := filetxn.Migrate(source, dest, dotenvToV1); err != nil {
			return fmt.Errorf("publish migrated configuration: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect migration source: %w", err)
	}
	if err := normalizeExistingConfiguration(dest); err != nil {
		return err
	}
	if err := filetxn.Migrate(source, dest, dotenvToV1); err != nil {
		return fmt.Errorf("publish migrated configuration: %w", err)
	}
	return nil
}

func normalizeExistingConfiguration(dest string) error {
	// Recover and validate any published destination before migration cleanup can retire its source.
	data, err := filetxn.Read(dest)
	switch {
	case err == nil:
		normalized, changed, err := normalizeLegacyYAML(data)
		if err != nil {
			return fmt.Errorf("normalize existing configuration: %w", err)
		}
		if _, err := current.Parse(normalized); err != nil {
			return fmt.Errorf("validate existing configuration: %w", err)
		}
		if !changed {
			return nil
		}
		if err := filestore.AtomicWrite(dest, normalized, 0o600); err != nil {
			return fmt.Errorf("publish normalized configuration: %w", err)
		}
		published, err := filetxn.Read(dest)
		if err != nil {
			return fmt.Errorf("verify normalized configuration: %w", err)
		}
		if !bytes.Equal(published, normalized) {
			return errors.New("normalized configuration verification failed")
		}
		if _, err := current.Parse(published); err != nil {
			return fmt.Errorf("validate normalized configuration: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("read existing configuration: %w", err)
	}
	return nil
}
