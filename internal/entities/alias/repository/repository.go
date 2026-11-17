// Package repository persists command aliases in the user's Zev directory.
package repository

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"gopkg.in/yaml.v3"
)

// Repository stores aliases in a YAML file.
type Repository struct {
	aliasesPath string
}

// New создаёт репозиторий алиасов. Директория передаётся вызывающим кодом
// (composition root), чтобы репозиторий не зависел от глобального состояния.
func New(zevDir string) *Repository {
	return &Repository{
		aliasesPath: filepath.Join(zevDir, "aliases"),
	}
}

// LoadAll reads all aliases and reports malformed YAML without modifying the file.
// Symlinks at the aliases file or application directory are rejected.
func (r *Repository) LoadAll() (aliases map[string]domain.Alias, err error) {
	if err := filestore.ValidateDirectory(filepath.Dir(r.aliasesPath)); err != nil {
		return nil, fmt.Errorf("validate aliases directory: %w", err)
	}
	err = filestore.WithLock(r.aliasesPath, func() error {
		aliases, err = r.loadAll()
		return err
	})
	return aliases, err
}

func (r *Repository) loadAll() (map[string]domain.Alias, error) {
	data, err := filestore.ReadFile(r.aliasesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]domain.Alias), nil
		}
		return nil, fmt.Errorf("failed to read aliases file: %w", err)
	}

	var raw map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse aliases: %w", err)
	}

	aliases := make(map[string]domain.Alias, len(raw))
	for name, template := range raw {
		aliases[name] = domain.Alias{
			Name:     name,
			Template: template,
		}
	}

	return aliases, nil
}

// Update loads, changes, and saves aliases under one cooperative transaction lock.
// If change returns an error, no data is saved. The callback must not call other
// repository methods for the same path. Load, callback, and write errors propagate,
// including filestore.ErrCommitted when replacement succeeded but sync failed.
func (r *Repository) Update(change func(map[string]domain.Alias) error) error {
	if err := filestore.ValidateDirectory(filepath.Dir(r.aliasesPath)); err != nil {
		return fmt.Errorf("validate aliases directory: %w", err)
	}
	if err := filestore.WithLock(r.aliasesPath, func() error {
		aliases, err := r.loadAll()
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
	raw := make(map[string]string, len(aliases))
	for name, alias := range aliases {
		raw[name] = alias.Template
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal aliases: %w", err)
	}

	if err := filestore.AtomicWrite(r.aliasesPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write aliases file: %w", err)
	}

	return nil
}
