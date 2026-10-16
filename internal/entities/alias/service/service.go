package service

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
)

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// Add добавляет новый алиас.
func (s *Service) Add(name, tmpl string) error {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return err
	}

	if _, exists := aliases[name]; exists {
		return fmt.Errorf("alias %q already exists", name)
	}

	aliases[name] = domain.Alias{
		Name:     name,
		Template: tmpl,
	}

	return s.repo.SaveAll(aliases)
}

// Remove удаляет алиас.
func (s *Service) Remove(name string) error {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return err
	}

	if _, exists := aliases[name]; !exists {
		return fmt.Errorf("alias %q not found", name)
	}

	delete(aliases, name)

	return s.repo.SaveAll(aliases)
}

// List возвращает все алиасы.
func (s *Service) List() ([]domain.Alias, error) {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return nil, err
	}

	result := make([]domain.Alias, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, alias)
	}

	return result, nil
}

// Resolve разрешает алиас с подстановкой аргументов.
func (s *Service) Resolve(name string, args []string) (string, error) {
	aliases, err := s.repo.LoadAll()
	if err != nil {
		return "", err
	}

	alias, exists := aliases[name]
	if !exists {
		return "", fmt.Errorf("alias %q not found", name)
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
