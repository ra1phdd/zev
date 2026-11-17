package main

import (
	"fmt"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/contextfile"
	"github.com/ra1phdd/zev/locale"
)

func projectContextSection(value string) string {
	if value == "" {
		return "[PROJECT_CONTEXT]\nNo project context files were supplied.\n[/PROJECT_CONTEXT]"
	}
	return value
}

func loadProjectContext(paths []string, allowSensitive bool) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf(locale.Text("resolve project context root: %w"), err)
	}
	content, err := contextfile.LoadWithOptions(root, paths, allowSensitive)
	if err != nil {
		return "", fmt.Errorf(locale.Text("load project context: %w"), err)
	}
	return content, nil
}
