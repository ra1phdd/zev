package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

// LoadUILanguage reads only the UI language with environment values taking
// precedence over the config file and the auto default. An empty environment
// ZEV_LANG is ignored, matching ordinary config loading. It does not validate
// provider or other settings, acquire locks, or create directories or files.
// Symlinks at the application directory or config file are rejected. It returns
// ErrInvalidHome before filesystem I/O when home is missing or relative; malformed
// config and invalid language errors omit file contents and configuration values.
func LoadUILanguage() (string, error) {
	path, err := getConfigPath()
	if err != nil {
		return "", err
	}
	if err := filestore.ValidateDirectory(filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("validate config directory: %w", err)
	}
	values, err := readValues(path)
	if err != nil {
		return "", err
	}
	value := values["ZEV_LANG"]
	if override, exists := os.LookupEnv("ZEV_LANG"); exists && strings.TrimSpace(override) != "" {
		value = override
	}
	value = normalizeUILanguage(value)
	if err := validateUILanguage(value); err != nil {
		return "", validationConfigError(err)
	}
	return value, nil
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
