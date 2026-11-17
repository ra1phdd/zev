// Package service manages user-defined executable scripts.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/editor"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

// ErrScriptNotFound indicates that a script is not registered in the metadata file.
var ErrScriptNotFound = errors.New("script not found")

// Service manages script metadata, files, and execution.
type Service struct{ repo *repository.Repository }

// New creates a script service backed by repo.
func New(repo *repository.Repository) *Service { return &Service{repo: repo} }

// CreateContext registers a script and opens an isolated editor copy unless
// SKIP_EDITOR is set to 1. Cancellation leaves an already-created script registered.
func (s *Service) CreateContext(ctx context.Context, name, description string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:`) {
		return fmt.Errorf("invalid script name %q", name)
	}
	script := domain.Script{Name: name, ScriptFile: name + ".sh", Description: description}
	comment := strings.ReplaceAll(strings.ReplaceAll(description, "\r\n", "\n"), "\r", "\n")
	content := []byte("#!/usr/bin/env bash\n\n# " + strings.ReplaceAll(comment, "\n", "\n# ") + "\n\n")
	if err := s.repo.Create(ctx, script, content); err != nil {
		return fmt.Errorf("create script: %w", err)
	}
	if os.Getenv("SKIP_EDITOR") != "1" {
		return s.OpenEditor(ctx, name)
	}
	return nil
}

// OpenEditor edits a private working copy, then atomically replaces the script
// only if it still names the inode originally opened. Editors using atomic saves
// are supported. Replaced or unsafe destinations are rejected without writing
// through their pathname.
func (s *Service) OpenEditor(ctx context.Context, name string) (err error) {
	script, err := s.findScript(ctx, name)
	if err != nil {
		return err
	}
	original, err := s.repo.OpenScript(ctx, script.ScriptFile)
	if err != nil {
		return fmt.Errorf("open script: %w", err)
	}
	defer closeResource(&err, original)
	directory, err := os.MkdirTemp("", "zev-editor-*")
	if err != nil {
		return fmt.Errorf("create editor directory: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(directory); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove editor directory: %w", cleanupErr))
		}
	}()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("open editor directory: %w", err)
	}
	defer closeResource(&err, root)
	working, err := root.OpenFile("script.sh", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create editor working file: %w", err)
	}
	_, copyErr := io.Copy(working, original)
	if err := errors.Join(copyErr, working.Close()); err != nil {
		return fmt.Errorf("copy script: %w", err)
	}
	if err := s.openInEditor(ctx, filepath.Join(directory, "script.sh")); err != nil {
		return err
	}
	data, err := repository.ReadEditorFile(root, "script.sh")
	if err != nil {
		return fmt.Errorf("read edited script: %w", err)
	}
	if err := s.repo.ReplaceScript(ctx, script.ScriptFile, original, data); err != nil {
		return fmt.Errorf("save edited script: %w", err)
	}
	return nil
}

func closeResource(result *error, resource io.Closer) {
	if err := resource.Close(); err != nil {
		*result = errors.Join(*result, fmt.Errorf("close script resource: %w", err))
	}
}

func (s *Service) openInEditor(ctx context.Context, path string) error {
	if err := editor.Open(ctx, path); err != nil {
		return fmt.Errorf("open script editor: %w", err)
	}
	return nil
}

// Remove removes script metadata and its confined file in one transaction.
func (s *Service) Remove(name string) error {
	if err := s.repo.Remove(name); err != nil {
		return fmt.Errorf("remove script: %w", err)
	}
	return nil
}

// List returns all validated script metadata.
func (s *Service) List() ([]domain.Script, error) {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load scripts: %w", err)
	}
	result := make([]domain.Script, 0, len(scripts))
	for _, script := range scripts {
		result = append(result, script)
	}
	return result, nil
}

func (s *Service) findScript(ctx context.Context, name string) (domain.Script, error) {
	scripts, err := s.repo.LoadAllContext(ctx)
	if err != nil {
		return domain.Script{}, fmt.Errorf("load scripts: %w", err)
	}
	script, ok := scripts[name]
	if !ok {
		return domain.Script{}, fmt.Errorf("%w: %q", ErrScriptNotFound, name)
	}
	return script, nil
}

// Show writes the exact body of a registered script to out without executing it.
// The script metadata is resolved before the pinned file is opened, and ctx is
// checked between reads. The caller owns out; Show closes the opened script.
func (s *Service) Show(ctx context.Context, name string, out io.Writer) (err error) {
	script, err := s.findScript(ctx, name)
	if err != nil {
		return err
	}
	file, err := s.repo.OpenScript(ctx, script.ScriptFile)
	if err != nil {
		return fmt.Errorf("open script: %w", err)
	}
	defer closeResource(&err, file)
	if _, err := io.Copy(out, contextReader{ctx: ctx, reader: file}); err != nil {
		return fmt.Errorf("show script %q: %w", name, err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("show script %q: %w", name, err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, fmt.Errorf("check context: %w", err)
	}
	read, err := r.reader.Read(p)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, fmt.Errorf("read context-bound output: %w", err)
	}
	return read, nil
}

// Execute runs the pinned script with literal arguments and the caller's stdin.
func (s *Service) Execute(ctx context.Context, name string, args []string) (err error) {
	script, err := s.findScript(ctx, name)
	if err != nil {
		return err
	}
	file, err := s.repo.OpenScript(ctx, script.ScriptFile)
	if err != nil {
		return fmt.Errorf("open script: %w", err)
	}
	defer closeResource(&err, file)
	if err := runScript(ctx, file, args); err != nil {
		return fmt.Errorf("execute script %q: %w", name, err)
	}
	return nil
}
