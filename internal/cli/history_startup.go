package cli

import (
	"context"
	"fmt"

	"github.com/ra1phdd/zev/internal/entities/history/repository"
	"github.com/ra1phdd/zev/internal/migrations/history"
)

func openHistoryStore(ctx context.Context, path string) (*repository.Store, error) {
	if err := history.Migrate(ctx, path, true); err != nil {
		return nil, fmt.Errorf("prepare history: %w", err)
	}
	store, err := repository.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite history: %w", err)
	}
	return store, nil
}
