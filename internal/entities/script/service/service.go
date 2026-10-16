package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/script/repository"
)

// ErrScriptNotFound возвращается, когда скрипт не зарегистрирован в scripts.yaml.
var ErrScriptNotFound = errors.New("script not found")

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// Create создаёт новый скрипт и открывает его в $EDITOR.
func (s *Service) Create(name, description string) error {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return err
	}

	if _, exists := scripts[name]; exists {
		return fmt.Errorf("script %q already exists", name)
	}

	scriptFile := name + ".sh"
	scriptPath := filepath.Join(s.repo.GetScriptsDir(), scriptFile)

	// Создаём директорию ~/.zev/scripts, если её нет
	if err := os.MkdirAll(s.repo.GetScriptsDir(), 0700); err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	// Создаём пустой скрипт с shebang
	content := "#!/usr/bin/env bash\n\n# " + description + "\n\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0700); err != nil {
		return fmt.Errorf("failed to create script file: %w", err)
	}

	// Добавляем метаданные в scripts.yaml
	scripts[name] = domain.Script{
		Name:        name,
		ScriptFile:  scriptFile,
		Description: description,
	}

	if err := s.repo.SaveAll(scripts); err != nil {
		return err
	}

	// Открываем скрипт в $EDITOR (пропускаем в тестах)
	if os.Getenv("SKIP_EDITOR") != "1" {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}

		cmd := exec.CommandContext(context.Background(), editor, scriptPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to open editor: %w", err)
		}
	}

	return nil
}

// Remove удаляет скрипт.
func (s *Service) Remove(name string) error {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return err
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

	return s.repo.SaveAll(scripts)
}

// List возвращает все скрипты.
func (s *Service) List() ([]domain.Script, error) {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return nil, err
	}

	result := make([]domain.Script, 0, len(scripts))
	for _, script := range scripts {
		result = append(result, script)
	}

	return result, nil
}

// Execute выполняет скрипт с аргументами.
func (s *Service) Execute(name string, args []string) error {
	scripts, err := s.repo.LoadAll()
	if err != nil {
		return err
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
	cmd := exec.CommandContext(context.Background(), "bash", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Ошибку не оборачиваем, чтобы main мог пробросить exit code скрипта
	return cmd.Run()
}
