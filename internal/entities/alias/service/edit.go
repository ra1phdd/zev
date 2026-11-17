package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/editor"
)

// Edit opens an existing alias template in the configured editor and saves the
// edited template only after the editor exits successfully.
func (s *Service) Edit(ctx context.Context, name string) (err error) {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return fmt.Errorf("load aliases: %w", err)
	}
	alias, ok := aliases[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrAliasNotFound, name)
	}
	temporary, err := os.CreateTemp("", "zev-alias-*.txt")
	if err != nil {
		return fmt.Errorf("create alias editor file: %w", err)
	}
	path := temporary.Name()
	defer func() {
		err = errors.Join(err, os.Remove(path))
	}()
	if _, err := temporary.WriteString(alias.Template + "\n"); err != nil {
		if closeErr := temporary.Close(); closeErr != nil {
			return fmt.Errorf("close alias editor file after write failure: %w", closeErr)
		}
		return fmt.Errorf("write alias editor file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close alias editor file: %w", err)
	}
	if err := editor.Open(ctx, path); err != nil {
		return fmt.Errorf("open alias editor: %w", err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // The path was created by os.CreateTemp and is not user-selected.
	if err != nil {
		return fmt.Errorf("read edited alias: %w", err)
	}
	template := strings.TrimSuffix(string(data), "\n")
	if strings.TrimSpace(template) == "" {
		return errors.New("edited alias template cannot be empty")
	}
	if err := s.repo.Update(func(all map[string]domain.Alias) error {
		current, exists := all[name]
		if !exists {
			return fmt.Errorf("%w: %q", ErrAliasNotFound, name)
		}
		current.Template = template
		all[name] = current
		return nil
	}); err != nil {
		return fmt.Errorf("save edited alias: %w", err)
	}
	return nil
}
