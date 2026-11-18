package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"gopkg.in/yaml.v3"
)

// LoadUILanguage reads the language without acquiring locks, changing files or
// validating unrelated provider settings. An absent config.yaml means auto.
func LoadUILanguage() (string, error) {
	path, err := getConfigPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := filestore.ValidateDirectory(dir); err != nil {
		return "", fmt.Errorf("validate config directory: %w", err)
	}
	data, err := filestore.ReadSingleLink(path, maxConfigBytes)
	if errors.Is(err, os.ErrNotExist) {
		return uiLanguageAuto, nil
	}
	if err != nil {
		return "", fmt.Errorf("read configuration language: %w", err)
	}
	root, err := currentDocument(data)
	if err != nil {
		return "", err
	}
	value := uiLanguageAuto
	seen := make(map[string]bool, len(root.Content)/2)
	for i := 0; i < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.ShortTag() != yamlString || seen[k.Value] {
			return "", errYAML
		}
		seen[k.Value] = true
		if k.Value == "language" {
			if v.ShortTag() != yamlString || v.Kind != yaml.ScalarNode {
				return "", errInvalidUILanguage
			}
			value = normalizeUILanguage(v.Value)
		}
	}
	return value, validateUILanguage(value)
}

const uiLanguageAuto = "auto"

func validateUILanguage(value string) error {
	switch normalizeUILanguage(value) {
	case uiLanguageAuto, "en", "ru":
		return nil
	default:
		return errInvalidUILanguage
	}
}

func normalizeUILanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return uiLanguageAuto
	}
	return value
}
