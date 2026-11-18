// Package repository persists command aliases in the user's Zev directory.
package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/metadata"
)

// Repository stores aliases in a versioned YAML file.
type Repository struct {
	aliasesPath string
}

// New creates an alias repository rooted at zevDir.
func New(zevDir string) *Repository {
	return &Repository{aliasesPath: filepath.Join(zevDir, "aliases")}
}

// LoadAll reads aliases, in the current format under the repository lock.
func (r *Repository) LoadAll() (map[string]domain.Alias, error) {
	return r.LoadAllContext(context.Background())
}

// LoadAllContext reads aliases under a cancellation-aware repository lock.
func (r *Repository) LoadAllContext(ctx context.Context) (map[string]domain.Alias, error) {
	if err := filestore.ValidateDirectory(filepath.Dir(r.aliasesPath)); err != nil {
		return nil, fmt.Errorf("validate aliases directory: %w", err)
	}
	var aliases map[string]domain.Alias
	if err := filestore.WithLockContext(ctx, r.aliasesPath, func() error {
		var err error
		aliases, err = r.load()
		return err
	}); err != nil {
		return nil, fmt.Errorf("load aliases: %w", err)
	}
	return aliases, nil
}

func (r *Repository) load() (map[string]domain.Alias, error) {
	source, err := filestore.ReadFileLimit(r.aliasesPath, 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]domain.Alias), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read aliases file: %w", err)
	}
	aliases, err := metadata.DecodeAliases(source)
	if err != nil {
		return nil, fmt.Errorf("decode aliases: %w", err)
	}
	return aliases, nil
}

// Update loads, changes, and saves canonical aliases under one transaction lock.
func (r *Repository) Update(change func(map[string]domain.Alias) error) error {
	if err := filestore.ValidateDirectory(filepath.Dir(r.aliasesPath)); err != nil {
		return fmt.Errorf("validate aliases directory: %w", err)
	}
	if err := filestore.WithLock(r.aliasesPath, func() error {
		aliases, err := r.load()
		if err != nil {
			return err
		}
		if err := change(aliases); err != nil {
			return err
		}
		return r.saveAll(aliases)
	}); err != nil {
		return fmt.Errorf("update aliases: %w", err)
	}
	return nil
}

func (r *Repository) saveAll(aliases map[string]domain.Alias) error {
	data, err := metadata.EncodeAliasesV2(aliases)
	if err != nil {
		return fmt.Errorf("encode aliases: %w", err)
	}
	if err := filestore.AtomicWrite(r.aliasesPath, data, 0o600); err != nil {
		return fmt.Errorf("write aliases file: %w", err)
	}
	return nil
}
