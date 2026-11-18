// Package migrations coordinates offline maintenance of Zev's independent stores.
// Normal startup opens only the stores required by the selected command.
package migrations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/migrations/aliases"
	configmigration "github.com/ra1phdd/zev/internal/migrations/config"
	"github.com/ra1phdd/zev/internal/migrations/history"
	"github.com/ra1phdd/zev/internal/migrations/scripts"
)

type store struct {
	name string
	run  func(context.Context, string) error
	path string
}

// Run upgrades stores in a stable order, continuing after independent storage
// errors. A missing home data directory is a no-op. Optional progress is called
// before each store; a reporting error or cancellation stops the run.
func Run(ctx context.Context, progress func(string) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve migration directory: %w", err)
	}
	if err := filestore.ValidateDirectory(dir); err != nil {
		return fmt.Errorf("validate migration directory: %w", err)
	}
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect migration directory: %w", err)
	}
	return runStores(ctx, progress, []store{
		{"configuration", configmigration.Migrate, dir},
		{"aliases", aliases.Migrate, dir},
		{"scripts", scripts.Migrate, dir},
		{"history", func(ctx context.Context, path string) error {
			return history.Migrate(ctx, path, false)
		}, filepath.Join(dir, "history.db")},
	})
}

func runStores(ctx context.Context, progress func(string) error, stores []store) error {
	var failures error
	for _, s := range stores {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, fmt.Errorf("run migrations: %w", err))
		}
		if progress != nil {
			if err := progress(s.name); err != nil {
				return errors.Join(failures, fmt.Errorf("report migration progress: %w", err))
			}
		}
		if err := s.run(ctx, s.path); err != nil {
			failures = errors.Join(failures, fmt.Errorf("migrate %s: %w", s.name, err))
		}
	}
	return failures
}
