// Package repository persists command aliases in the user's Zev directory.
package repository

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/core/metadatastore"
)

// ErrMigrationRequired indicates that a legacy aliases file must be migrated
// before a mutation can safely publish canonical metadata.
var ErrMigrationRequired = errors.New("alias metadata migration required")

// Repository stores aliases in a versioned YAML file.
type Repository struct {
	aliasesPath string
}

// New creates an alias repository rooted at zevDir.
func New(zevDir string) *Repository {
	return &Repository{aliasesPath: filepath.Join(zevDir, "aliases")}
}

// LoadAll reads canonical aliases or safely decodes a legacy file without
// modifying it. Legacy data is read-only until the migration slice is present.
func (r *Repository) LoadAll() (map[string]domain.Alias, error) {
	if err := filestore.ValidateDirectory(filepath.Dir(r.aliasesPath)); err != nil {
		return nil, fmt.Errorf("validate aliases directory: %w", err)
	}
	var aliases map[string]domain.Alias
	if err := filestore.WithLock(r.aliasesPath, func() error {
		var err error
		aliases, err = r.loadAndMigrate()
		return err
	}); err != nil {
		return nil, fmt.Errorf("load aliases: %w", err)
	}
	return aliases, nil
}

func (r *Repository) loadAll() (map[string]domain.Alias, bool, error) {
	data, err := filestore.ReadFileLimit(r.aliasesPath, 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]domain.Alias), false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read aliases file: %w", err)
	}
	aliases, legacy, err := metadata.DecodeAliases(data)
	if err != nil {
		return nil, false, fmt.Errorf("decode aliases: %w", err)
	}
	return aliases, legacy, nil
}

func validateMigrationMarkerSize(journal string) error {
	_, err := filestore.ReadFileLimit(filepath.Join(journal, "marker"), 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read migration marker: %w", err)
	}
	return nil
}

func (r *Repository) loadAndMigrate() (map[string]domain.Alias, error) {
	journal := filepath.Join(filepath.Dir(r.aliasesPath), ".aliases-migration")
	if err := validateMigrationMarkerSize(journal); err != nil {
		return nil, fmt.Errorf("validate aliases migration marker: %w", err)
	}
	if err := metadatastore.Recover(r.aliasesPath, journal); err != nil {
		return nil, fmt.Errorf("recover aliases migration: %w", err)
	}
	aliases, legacy, err := r.loadAll()
	if err != nil || !legacy {
		return aliases, err
	}
	canonical, err := metadata.EncodeAliases(aliases)
	if err != nil {
		return nil, fmt.Errorf("encode aliases migration: %w", err)
	}
	legacyData, err := filestore.ReadFileLimit(r.aliasesPath, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("read aliases migration source: %w", err)
	}
	if err := metadatastore.Migrate(r.aliasesPath, journal, legacyData, canonical); err != nil {
		return nil, fmt.Errorf("migrate aliases: %w", err)
	}
	aliases, legacy, err = r.loadAll()
	if err != nil {
		return nil, err
	}
	if legacy {
		return nil, errors.New("aliases migration completed without canonical metadata")
	}
	return aliases, nil
}

// Update loads, changes, and saves canonical aliases under one transaction lock.
// Legacy data is migrated before the callback runs.
func (r *Repository) Update(change func(map[string]domain.Alias) error) error {
	if err := filestore.ValidateDirectory(filepath.Dir(r.aliasesPath)); err != nil {
		return fmt.Errorf("validate aliases directory: %w", err)
	}
	if err := filestore.WithLock(r.aliasesPath, func() error {
		aliases, err := r.loadAndMigrate()
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
	data, err := metadata.EncodeAliases(aliases)
	if err != nil {
		return fmt.Errorf("encode aliases: %w", err)
	}
	if err := filestore.AtomicWrite(r.aliasesPath, data, 0o600); err != nil {
		return fmt.Errorf("write aliases file: %w", err)
	}
	if err := metadatastore.ClearCompleted(filepath.Join(filepath.Dir(r.aliasesPath), ".aliases-migration")); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("clear aliases migration marker: %w", err))
	}
	return nil
}
