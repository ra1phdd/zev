// Package service manages user-defined executable scripts.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

// ErrScriptNotFound indicates that a script is not registered in the metadata file.
var ErrScriptNotFound = errors.New("script not found")

// Service manages script metadata, files, and execution.
type Service struct {
	repo *repository.Repository
}

// New creates a script service backed by repo.
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// Create создаёт новый скрипт и открывает его в редакторе.
func (s *Service) Create(name, description string) error {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return fmt.Errorf("load scripts: %w", err)
	}

	if _, exists := scripts[name]; exists {
		return fmt.Errorf("script %q already exists", name)
	}

	scriptFile := name + ".sh"
	scriptPath := filepath.Join(s.repo.GetScriptsDir(), scriptFile)

	// Создаём директорию ~/.zev/scripts, если её нет
	if err := os.MkdirAll(s.repo.GetScriptsDir(), 0o700); err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	// Создаём пустой скрипт с shebang
	content := "#!/usr/bin/env bash\n\n# " + description + "\n\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0o700); err != nil { //nolint:gosec // Script files must be owner-executable for direct use and remain private.
		return fmt.Errorf("failed to create script file: %w", err)
	}

	// Добавляем метаданные в scripts.yaml
	scripts[name] = domain.Script{
		Name:        name,
		ScriptFile:  scriptFile,
		Description: description,
	}

	if err := s.repo.SaveAll(scripts); err != nil {
		return fmt.Errorf("save scripts: %w", err)
	}

	// Открываем скрипт в редакторе (пропускаем в тестах)
	if os.Getenv("SKIP_EDITOR") != "1" {
		if err := s.openInEditor(scriptPath); err != nil {
			return fmt.Errorf("open script in editor: %w", err)
		}
	}

	return nil
}

// openInEditor opens the script in the configured editor.
func (s *Service) openInEditor(path string) error {
	editor := s.chooseEditor()

	// Если редактор не найден, выводим инструкцию
	if editor == "" {
		return fmt.Errorf("no editor found (nano, vim, vi, or $EDITOR/$SCRIPT_EDITOR not set).\nPlease create a bash script manually:\n  %s", path)
	}

	// The editor path was validated with exec.LookPath in chooseEditor.
	cmd := exec.CommandContext(context.Background(), editor, path) //nolint:gosec // The editor is accepted only after executable-path validation.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	return nil
}

// chooseEditor выбирает редактор по приоритету.
func (s *Service) chooseEditor() string {
	// 1. Проверяем $SCRIPT_EDITOR из конфига
	if editor := os.Getenv("SCRIPT_EDITOR"); editor != "" {
		if _, err := exec.LookPath(editor); err == nil {
			return editor
		}
	}

	// 2. Проверяем $EDITOR
	if editor := os.Getenv("EDITOR"); editor != "" {
		if _, err := exec.LookPath(editor); err == nil {
			return editor
		}
	}

	// 3. Fallback на nano, vim, vi по порядку
	fallbacks := []string{"nano", "vim", "vi"}
	for _, editor := range fallbacks {
		if _, err := exec.LookPath(editor); err == nil {
			return editor
		}
	}

	return ""
}

// Remove удаляет скрипт.
func (s *Service) Remove(name string) error {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return fmt.Errorf("load scripts: %w", err)
	}

	script, exists := scripts[name]
	if !exists {
		return fmt.Errorf("script %q not found", name)
	}

	// Удаляем файл скрипта
	scriptPath := filepath.Join(s.repo.GetScriptsDir(), script.ScriptFile)
	if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove script file: %w", err)
	}

	// Удаляем метаданные
	delete(scripts, name)

	if err := s.repo.SaveAll(scripts); err != nil {
		return fmt.Errorf("save scripts: %w", err)
	}

	return nil
}

// List возвращает все скрипты.
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

// Execute executes a saved script with arguments.
func (s *Service) Execute(ctx context.Context, name string, args []string) error {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return fmt.Errorf("load scripts: %w", err)
	}

	script, exists := scripts[name]
	if !exists {
		return fmt.Errorf("%w: %q", ErrScriptNotFound, name)
	}

	scriptPath := filepath.Join(s.repo.GetScriptsDir(), script.ScriptFile)

	// Проверяем существование файла
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("script file not found: %w", err)
	}

	// Выполняем скрипт через bash
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(ctx, "bash", cmdArgs...) //nolint:gosec // bash is fixed; scriptPath and args are passed as argv, not shell source.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute script %q: %w", name, err)
	}

	return nil
}
