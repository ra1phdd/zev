// Package service manages user-defined executable scripts.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/editor"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

// ErrScriptNotFound indicates that a script is not registered in the metadata file.
var ErrScriptNotFound = errors.New("script not found")

const maxExecutionBodyBytes = 4 << 20

// Service manages script metadata, files, and execution.
type Service struct{ repo *repository.Repository }

// New creates a script service backed by repo.
func New(repo *repository.Repository) *Service { return &Service{repo: repo} }

// OpenEditor captures the registered source before launching an isolated editor.
// Source drift returns ErrScriptChanged without publishing the stale edit.
func (s *Service) OpenEditor(ctx context.Context, name string) error {
	snapshot, err := s.Snapshot(ctx, name)
	if err != nil {
		return err
	}
	return s.EditBody(ctx, snapshot.Body)
}

// EditBody opens a private copy of source and publishes only while it remains
// current. Atomic editor saves are supported; stale copies return ErrScriptChanged.
func (s *Service) EditBody(ctx context.Context, source repository.BodySnapshot) (err error) {
	body, err := s.repo.ReadBody(ctx, repository.ReadBodyParams{Snapshot: source})
	if err != nil {
		return fmt.Errorf("read editor source: %w", err)
	}
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
	_, copyErr := working.Write(body)
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
	if err := s.repo.ReplaceScript(ctx, repository.ReplaceParams{Source: source, Body: data}); err != nil {
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
	slices.SortFunc(result, func(a, b domain.Script) int { return strings.Compare(a.Name, b.Name) })
	return result, nil
}

// Find returns the validated metadata for a named script without opening or
// executing its body. Use errors.Is with ErrScriptNotFound for absence.
func (s *Service) Find(ctx context.Context, name string) (domain.Script, error) {
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
	script, err := s.Find(ctx, name)
	if err != nil {
		return err
	}
	file, err := s.repo.OpenScriptPinned(ctx, script, nil)
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

// Execute runs an immutable snapshot of the registered script with literal arguments.
func (s *Service) Execute(ctx context.Context, name string, args []string) error {
	return s.ExecuteAuthorized(ctx, name, args, nil)
}

// ExecuteAuthorized snapshots the exact script bytes before authorization and
// executes that snapshot only after authorize approves the same bytes. The source
// path may change after snapshotting without changing the executed program.
func (s *Service) ExecuteAuthorized(ctx context.Context, name string, args []string, authorize func(domain.Script, []byte) error) (err error) {
	script, err := s.Find(ctx, name)
	if err != nil {
		return err
	}
	file, err := s.repo.OpenScriptPinned(ctx, script, func(current domain.Script) error {
		script = current
		for i, argument := range current.Arguments {
			if argument.Required && i >= len(args) {
				return fmt.Errorf("script %q requires argument %d: %s — %s", name, i+1, argument.Name, argument.Description)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("open script: %w", err)
	}
	defer closeResource(&err, file)
	body, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: file}, maxExecutionBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read script snapshot: %w", err)
	}
	if len(body) > maxExecutionBodyBytes {
		return errors.New("script body exceeds execution limit")
	}
	if err := authorizeScript(authorize, script, body); err != nil {
		return err
	}
	executionFile, err := os.CreateTemp("", "zev-script-exec-*")
	if err != nil {
		return fmt.Errorf("create execution snapshot: %w", err)
	}
	defer func() {
		if removeErr := os.Remove(executionFile.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove execution snapshot: %w", removeErr))
		}
	}()
	defer closeResource(&err, executionFile)
	if err := executionFile.Chmod(0o700); err != nil {
		return fmt.Errorf("set execution snapshot permissions: %w", err)
	}
	if _, err := executionFile.Write(body); err != nil {
		return fmt.Errorf("write execution snapshot: %w", err)
	}
	if _, err := executionFile.Seek(0, 0); err != nil {
		return fmt.Errorf("rewind execution snapshot: %w", err)
	}
	if err := runScript(ctx, executionFile, args); err != nil {
		return fmt.Errorf("execute script %q: %w", name, err)
	}
	return nil
}

func authorizeScript(authorize func(domain.Script, []byte) error, script domain.Script, body []byte) error {
	if authorize == nil {
		return nil
	}
	if err := authorize(script, append([]byte(nil), body...)); err != nil {
		return fmt.Errorf("authorize script snapshot: %w", err)
	}
	return nil
}
