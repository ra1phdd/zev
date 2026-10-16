package repository

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"gopkg.in/yaml.v3"
)

type scriptYAML struct {
	Script      string `yaml:"script"`
	Description string `yaml:"description"`
}

type Repository struct {
	scriptsYAMLPath string
	scriptsDir      string
}

// New создаёт репозиторий скриптов. Метаданные хранятся в файле ~/.zev/scripts,
// сами скрипты — в каталоге ~/.zev/script_files (пути не должны совпадать,
// иначе файл и каталог не могут сосуществовать).
func New() *Repository {
	zevDir := config.GetZevDir()
	return &Repository{
		scriptsYAMLPath: filepath.Join(zevDir, "scripts"),
		scriptsDir:      filepath.Join(zevDir, "script_files"),
	}
}

// LoadAll загружает все метаданные скриптов из ~/.zev/scripts.
func (r *Repository) LoadAll() (map[string]domain.Script, error) {
	data, err := os.ReadFile(r.scriptsYAMLPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]domain.Script), nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", r.scriptsYAMLPath, err)
	}

	var raw map[string]scriptYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", r.scriptsYAMLPath, err)
	}

	scripts := make(map[string]domain.Script, len(raw))
	for name, meta := range raw {
		scripts[name] = domain.Script{
			Name:        name,
			ScriptFile:  meta.Script,
			Description: meta.Description,
		}
	}

	return scripts, nil
}

// SaveAll сохраняет все метаданные скриптов в ~/.zev/scripts.
func (r *Repository) SaveAll(scripts map[string]domain.Script) error {
	if err := os.MkdirAll(filepath.Dir(r.scriptsYAMLPath), 0700); err != nil {
		return fmt.Errorf("failed to create scripts dir: %w", err)
	}

	raw := make(map[string]scriptYAML, len(scripts))
	for name, script := range scripts {
		raw[name] = scriptYAML{
			Script:      script.ScriptFile,
			Description: script.Description,
		}
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal scripts: %w", err)
	}

	if err := os.WriteFile(r.scriptsYAMLPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", r.scriptsYAMLPath, err)
	}

	return nil
}

// GetScriptsDir возвращает путь к директории ~/.zev/script_files.
func (r *Repository) GetScriptsDir() string {
	return r.scriptsDir
}
