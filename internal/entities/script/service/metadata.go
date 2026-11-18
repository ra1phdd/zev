package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

// ErrScriptChanged indicates a stale record, body, or selected metadata field.
var ErrScriptChanged = repository.ErrBodyChanged

// Patch selects metadata fields for replacement. Nil fields are preserved.
type Patch struct {
	Description *string
	Arguments   *[]domain.Argument
}

// Create registers a validated complete model with the supplied body, without an editor.
func (s *Service) Create(ctx context.Context, script domain.Script, body []byte) error {
	if err := metadata.ValidateScriptFields(script); err != nil {
		return fmt.Errorf("invalid script: %w", err)
	}
	if err := s.repo.Create(ctx, script, body); err != nil {
		return fmt.Errorf("create script: %w", err)
	}
	return nil
}

// Snapshot carries detached metadata and an immutable body prerequisite.
type Snapshot struct {
	Script domain.Script
	Body   repository.BodySnapshot
}

// Snapshot captures metadata and body identity before user interaction or generation.
// Record drift between lookup and capture returns ErrScriptChanged.
func (s *Service) Snapshot(ctx context.Context, name string) (Snapshot, error) {
	script, body, err := s.repo.CaptureScript(ctx, name)
	if err != nil {
		return Snapshot{}, fmt.Errorf("capture script source: %w", err)
	}
	return Snapshot{Script: script, Body: body}, nil
}

// Patch compares immutable identity and selected metadata under the repository lock.
// Unrelated records and unselected fields do not cause conflicts.
func (s *Service) Patch(ctx context.Context, snapshot Snapshot, patch Patch) (domain.Script, error) {
	if !snapshot.Body.Matches(snapshot.Script) {
		return domain.Script{}, fmt.Errorf("%w: mismatched body prerequisite", ErrScriptChanged)
	}
	var result domain.Script
	err := s.repo.Update(ctx, repository.UpdateParams{Source: &snapshot.Body, Change: func(all map[string]domain.Script) error {
		current, ok := all[snapshot.Script.Name]
		if !ok || current.RecordID != snapshot.Script.RecordID || current.ScriptFile != snapshot.Script.ScriptFile {
			return fmt.Errorf("%w: %q", ErrScriptChanged, snapshot.Script.Name)
		}
		if patch.Description != nil && current.Description != snapshot.Script.Description || patch.Arguments != nil && !slices.Equal(current.Arguments, snapshot.Script.Arguments) {
			return fmt.Errorf("%w: %q", ErrScriptChanged, snapshot.Script.Name)
		}
		result = current
		if patch.Description != nil {
			result.Description = *patch.Description
		}
		if patch.Arguments != nil {
			result.Arguments = *patch.Arguments
		}
		if err := metadata.ValidateScript(result); err != nil {
			return fmt.Errorf("validate script patch: %w", err)
		}
		all[result.Name] = result
		return nil
	}})
	if err != nil {
		return domain.Script{}, fmt.Errorf("patch script: %w", err)
	}
	return result, nil
}

// ReplaceBody replaces only the supplied source body; metadata remains unchanged.
// Stale record identities, inodes or content return ErrScriptChanged.
func (s *Service) ReplaceBody(ctx context.Context, params repository.ReplaceParams) error {
	if err := s.repo.ReplaceScript(ctx, params); err != nil {
		return fmt.Errorf("replace script body: %w", err)
	}
	return nil
}
