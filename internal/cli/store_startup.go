package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/filestore"
	configmigration "github.com/ra1phdd/zev/internal/migrations/config"
)

// zevDirectory checks existence without creating lock files for an absent store.
func zevDirectory() (string, bool, error) {
	dir, err := config.GetZevDir()
	if err != nil {
		return "", false, fmt.Errorf("resolve Zev directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return dir, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect Zev directory: %w", err)
	}
	if !info.IsDir() {
		return "", false, errors.New("storage directory must not be a symlink or special file")
	}
	return dir, true, nil
}

// prepareStore returns the path for a subsequent repository operation, even
// when the writer will be initializing a store for the first time.
func prepareStore(ctx context.Context, migrate func(context.Context, string) error) (string, error) {
	dir, present, err := zevDirectory()
	if err != nil || !present {
		return dir, err
	}
	return dir, migrate(ctx, dir)
}

func loadConfig(ctx context.Context) (*config.Config, error) {
	dir, present, err := zevDirectory()
	if err != nil {
		return nil, err
	}
	if !present {
		return config.DefaultConfig(), nil
	}
	if err := configmigration.Migrate(ctx, dir); err != nil {
		return nil, fmt.Errorf("migrate configuration: %w", err)
	}
	cfg, err := config.LoadContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}

// Help and diagnostics inspect an atomic snapshot, never acquiring a writer
// lock, running recovery, opening script bodies or creating an absent store.
func readStoreSnapshot[T any](dir, filename string, decode func([]byte) (T, error)) (T, error) {
	var zero T
	data, err := filestore.ReadSingleLink(filepath.Join(dir, filename), 1<<20)
	if err != nil {
		return zero, fmt.Errorf("inspect %s: %w", filename, err)
	}
	return decode(data)
}

func doctorConfig() (*config.Config, error) {
	dir, present, err := zevDirectory()
	if err != nil {
		return nil, err
	}
	if !present {
		return config.DefaultConfig(), nil
	}
	if err := config.ValidateFileSecurity(filepath.Join(dir, "config.yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("validate configuration security: %w", err)
	}
	cfg, err := readStoreSnapshot(dir, "config.yaml", config.Parse)
	if errors.Is(err, os.ErrNotExist) {
		if _, err := os.Lstat(filepath.Join(dir, "config")); errors.Is(err, os.ErrNotExist) {
			return config.DefaultConfig(), nil
		}
		return nil, errors.New("configuration needs attention; run zev update --migrate-only")
	}
	return cfg, err
}
