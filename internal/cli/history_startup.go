package cli

import (
	"context"
	"fmt"

	"github.com/ra1phdd/zev/internal/entities/history/migrator"
	"github.com/ra1phdd/zev/internal/entities/history/repository"
)

func openHistoryStore(ctx context.Context, path string) (*repository.Store, error) {
	if err := migrator.Migrate(ctx, path); err != nil {
		return nil, fmt.Errorf("migrate legacy history: %w", err)
	}
	store, err := repository.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite history: %w", err)
	}
	return store, nil
}
