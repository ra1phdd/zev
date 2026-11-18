// Package service manages command aliases and renders argv-safe templates.
package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
)

// ErrAliasNotFound indicates that no alias exists for a requested name.
var ErrAliasNotFound = errors.New("alias not found")

// ErrAliasExists indicates that Add would overwrite an existing alias.
var ErrAliasExists = errors.New("alias already exists")

// ErrMigrationRequired indicates that a legacy aliases file must be migrated
// before it can be changed.
var ErrMigrationRequired = repository.ErrMigrationRequired

// Service manages command aliases and their metadata.
type Service struct {
	repo *repository.Repository
}

// New creates an alias service backed by repo.
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// AddAlias adds a complete alias model in a single read-modify-write transaction.
func (s *Service) AddAlias(alias domain.Alias) error {
	if err := s.repo.Update(func(aliases map[string]domain.Alias) error {
		if _, exists := aliases[alias.Name]; exists {
			return fmt.Errorf("%w: %q", ErrAliasExists, alias.Name)
		}
		if err := metadata.ValidateAlias(alias); err != nil {
			return fmt.Errorf("invalid alias: %w", err)
		}
		aliases[alias.Name] = alias
		return nil
	}); err != nil {
		return fmt.Errorf("add alias: %w", err)
	}
	return nil
}

// Add adds a new alias with empty metadata.
func (s *Service) Add(name, tmpl string) error {
	return s.AddAlias(domain.Alias{Name: name, Template: tmpl})
}

// AddCommand stores a history command as a canonical alias with empty metadata.
func (s *Service) AddCommand(name, command string) error {
	return s.Add(name, command)
}

// Remove removes an alias in one read-modify-write transaction.
func (s *Service) Remove(name string) error {
	if err := s.repo.Update(func(aliases map[string]domain.Alias) error {
		if _, exists := aliases[name]; !exists {
			return fmt.Errorf("%w: %q", ErrAliasNotFound, name)
		}
		delete(aliases, name)
		return nil
	}); err != nil {
		return fmt.Errorf("remove alias: %w", err)
	}
	return nil
}

// List returns aliases in stable name order.
func (s *Service) List() ([]domain.Alias, error) {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load aliases: %w", err)
	}
	result := make([]domain.Alias, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, alias)
	}
	slices.SortFunc(result, func(a, b domain.Alias) int { return strings.Compare(a.Name, b.Name) })
	return result, nil
}

// Resolve renders an alias without executing it and preserves argv boundaries.
func (s *Service) Resolve(name string, args []string) (string, error) {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return "", fmt.Errorf("load aliases: %w", err)
	}
	alias, exists := aliases[name]
	if !exists {
		return "", fmt.Errorf("%w: %q", ErrAliasNotFound, name)
	}
	resolved, err := metadata.RenderAlias(alias, args)
	if err != nil {
		return "", fmt.Errorf("render alias %q: %w", name, err)
	}
	return resolved, nil
}
