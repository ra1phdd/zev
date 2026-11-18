package service

import (
	"fmt"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// Patch is a field-level alias edit. Nil fields are preserved.
type Patch struct {
	Template    *string
	Description *string
	Arguments   *[]domain.Argument
}

// Patch updates only selected fields and compares the complete selected snapshot.
func (s *Service) Patch(snapshot domain.Alias, patch Patch) (domain.Alias, error) {
	var result domain.Alias
	err := s.repo.Update(func(all map[string]domain.Alias) error {
		current, ok := all[snapshot.Name]
		if !ok {
			return fmtNotFound(snapshot.Name)
		}
		if patch.Template != nil && current.Template != snapshot.Template || patch.Description != nil && current.Description != snapshot.Description || patch.Arguments != nil && !sameArgs(current.Arguments, snapshot.Arguments) {
			return fmt.Errorf("%w: %q", ErrAliasChanged, snapshot.Name)
		}
		result = current
		if patch.Template != nil {
			result.Template = *patch.Template
		}
		if patch.Description != nil {
			result.Description = *patch.Description
		}
		if patch.Arguments != nil {
			result.Arguments = *patch.Arguments
		}
		all[result.Name] = result
		return nil
	})
	if err != nil {
		return domain.Alias{}, fmt.Errorf("patch alias: %w", err)
	}
	return result, nil
}
func fmtNotFound(name string) error { return fmt.Errorf("%w: %q", ErrAliasNotFound, name) }
func sameArgs(a, b []domain.Argument) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
