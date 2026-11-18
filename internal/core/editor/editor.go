// Package editor resolves and launches the user's preferred text editor.
package editor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/childprocess"
	"github.com/ra1phdd/zev/internal/core/config"
)

// Resolve returns the configured editor, then the first available editor from
// SCRIPT_EDITOR, EDITOR, nano, vim and vi. An empty result means no supported
// editor was found.
func Resolve(ctx context.Context) string {
	if name := configuredEditor(ctx); name != "" {
		return name
	}
	for _, candidate := range []string{os.Getenv("SCRIPT_EDITOR"), os.Getenv("EDITOR"), "nano", "vim", "vi"} {
		if candidate == "" {
			continue
		}
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func configuredEditor(ctx context.Context) string {
	dir, err := config.GetZevDir()
	if err != nil {
		return ""
	}
	info, err := os.Lstat(filepath.Join(dir, "config.yaml"))
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	cfg, err := config.LoadContext(ctx)
	if err != nil || cfg.ScriptEditor == "" {
		return ""
	}
	if _, err := exec.LookPath(cfg.ScriptEditor); err != nil {
		return ""
	}
	return cfg.ScriptEditor
}

// Open launches the resolved editor for path and inherits the current terminal.
func Open(ctx context.Context, path string) error {
	name := Resolve(ctx)
	if name == "" {
		return errors.New("no editor found (nano, vim, vi, or $EDITOR/$SCRIPT_EDITOR not set)")
	}
	cmd := exec.CommandContext(ctx, name, path) //nolint:gosec // The editor is selected from explicit user configuration or known binaries.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := childprocess.Run(ctx, cmd); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}
