package repository

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"gopkg.in/yaml.v3"
)

type Repository struct {
	aliasesPath string
}

func New() *Repository {
	return &Repository{
		aliasesPath: filepath.Join(config.GetZevDir(), "aliases"),
	}
}

// LoadAll загружает все алиасы из файла ~/.zev/aliases.
func (r *Repository) LoadAll() (map[string]domain.Alias, error) {
	data, err := os.ReadFile(r.aliasesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]domain.Alias), nil
		}
		return nil, fmt.Errorf("failed to read aliases file: %w", err)
	}

	var raw map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse aliases: %w", err)
	}

	aliases := make(map[string]domain.Alias, len(raw))
	for name, template := range raw {
		aliases[name] = domain.Alias{
			Name:     name,
			Template: template,
		}
	}

	return aliases, nil
}

// SaveAll сохраняет все алиасы в файл ~/.zev/aliases.
func (r *Repository) SaveAll(aliases map[string]domain.Alias) error {
	if err := os.MkdirAll(filepath.Dir(r.aliasesPath), 0700); err != nil {
		return fmt.Errorf("failed to create aliases dir: %w", err)
	}

	raw := make(map[string]string, len(aliases))
	for name, alias := range aliases {
		raw[name] = alias.Template
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal aliases: %w", err)
	}

	if err := os.WriteFile(r.aliasesPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write aliases file: %w", err)
	}

	return nil
}
