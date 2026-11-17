package migrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/ra1phdd/zev/internal/entities/history/repository"
)

type Store struct {
	*repository.Store
	*migrator
}

func Open(ctx context.Context, path string) (*Store, error) {
	store, err := repository.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open test store: %w", err)
	}
	db, err := repository.OpenDatabase(ctx, path, false)
	if err != nil {
		if closeErr := store.Close(); closeErr != nil {
			return nil, errors.Join(fmt.Errorf("open test database: %w", err), closeErr)
		}
		return nil, fmt.Errorf("open test database: %w", err)
	}
	return &Store{Store: store, migrator: &migrator{db: db}}, nil
}

func (s *Store) Close() error {
	return errors.Join(s.Store.Close(), s.db.Close())
}
