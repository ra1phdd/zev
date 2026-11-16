// Package service manages command aliases and resolves their argument templates.
package service

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
)

// ErrAliasNotFound indicates that Resolve found no alias for the requested name.
// Use errors.Is to distinguish absence from storage or template failures.
var ErrAliasNotFound = errors.New("alias not found")

// ErrAliasExists indicates that Add would overwrite an existing alias.
var ErrAliasExists = errors.New("alias already exists")

// Service manages command aliases and resolves their argument templates.
type Service struct {
	repo *repository.Repository
}

// New creates an alias service backed by repo.
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// Add adds a new alias in a single read-modify-write transaction.
func (s *Service) Add(name, tmpl string) error {
	if err := s.repo.Update(func(aliases map[string]domain.Alias) error {
		if _, exists := aliases[name]; exists {
			return fmt.Errorf("%w: %q", ErrAliasExists, name)
		}
		aliases[name] = domain.Alias{Name: name, Template: tmpl}
		return nil
	}); err != nil {
		return fmt.Errorf("add alias: %w", err)
	}
	return nil
}

// AddCommand stores a literal command using the existing alias template format.
// Template delimiters in shell commands are escaped so Resolve preserves them.
func (s *Service) AddCommand(name, command string) error {
	literal := strings.NewReplacer("{{", `{{"{{"}}`, "}}", `{{"}}"}}`).Replace(command)
	if err := s.Add(name, literal); err != nil {
		return fmt.Errorf("add command alias: %w", err)
	}
	return nil
}

// Remove removes an alias in a single read-modify-write transaction.
func (s *Service) Remove(name string) error {
	if err := s.repo.Update(func(aliases map[string]domain.Alias) error {
		if _, exists := aliases[name]; !exists {
			return fmt.Errorf("alias %q not found", name)
		}
		delete(aliases, name)
		return nil
	}); err != nil {
		return fmt.Errorf("remove alias: %w", err)
	}
	return nil
}

// List возвращает все алиасы.
func (s *Service) List() ([]domain.Alias, error) {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load aliases: %w", err)
	}

	result := make([]domain.Alias, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, alias)
	}

	return result, nil
}

// Resolve substitutes args into the named alias template. An absent alias wraps
// ErrAliasNotFound; storage, parsing, and template errors do not.
func (s *Service) Resolve(name string, args []string) (string, error) {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return "", fmt.Errorf("load aliases: %w", err)
	}

	alias, exists := aliases[name]
	if !exists {
		return "", fmt.Errorf("%w: %q", ErrAliasNotFound, name)
	}

	// Проверяем, использует ли шаблон переменные для аргументов
	hasArgVars := strings.Contains(alias.Template, "{{.args}}") ||
		strings.Contains(alias.Template, "{{.arg")

	tmpl, err := template.New(name).Parse(alias.Template)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	data := buildTemplateData(args)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	result := buf.String()

	// Если шаблон не использует переменные и есть аргументы — добавляем их в конец
	if !hasArgVars && len(args) > 0 {
		result = result + " " + strings.Join(args, " ")
	}

	return result, nil
}

// buildTemplateData строит данные для шаблона.
func buildTemplateData(args []string) map[string]any {
	data := make(map[string]any)

	// Позиционные аргументы: {{.arg1}}, {{.arg2}}, ...
	for i, arg := range args {
		key := fmt.Sprintf("arg%d", i+1)
		data[key] = arg
	}

	// Все аргументы через пробел: {{.args}}
	allArgs := ""
	var allArgsSb109 strings.Builder
	for i, arg := range args {
		if i > 0 {
			allArgsSb109.WriteString(" ")
		}
		allArgsSb109.WriteString(arg)
	}
	allArgs += allArgsSb109.String()
	data["args"] = allArgs

	return data
}
