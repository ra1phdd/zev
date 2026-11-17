//go:build e2e

package e2e_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	homeStorageRelative     = "relative"
	homeStorageRelativePath = "relative-home"
	homeStorageAlias        = "absolute-alias"
)

func TestCLIStorageRequiresAbsoluteHome(t *testing.T) {
	t.Parallel()
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("alias fixture requires sh")
	}
	for _, name := range []string{"unset", "empty", homeStorageRelative, "dot", "absolute", homeStorageAlias} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			home := ""
			switch name {
			case homeStorageRelative:
				home = homeStorageRelativePath
			case "dot":
				home = "."
			case "absolute", homeStorageAlias:
				home = t.TempDir()
				if name == homeStorageAlias {
					target := home
					home = filepath.Join(project, "trusted-home")
					if err := os.Symlink(target, home); err != nil {
						t.Skipf("home aliases unavailable: %v", err)
					}
				}
			}
			projectRoot := filepath.Join(project, ".zev")
			if name == homeStorageRelative {
				projectRoot = filepath.Join(project, home, ".zev")
			}
			projectMarker := filepath.Join(project, "project-alias-executed")
			writeHomeStorageAlias(t, projectRoot, projectMarker)
			trustedMarker := filepath.Join(project, "trusted-alias-executed")
			if filepath.IsAbs(home) {
				writeHomeStorageAlias(t, filepath.Join(home, ".zev"), trustedMarker)
			}
			cmd := cliCommand(t, home, "storagefixture")
			cmd.Dir = project
			setCLIEnv(cmd, map[string]string{"SHELL": shell})
			if name == "unset" {
				filtered := cmd.Env[:0]
				for _, entry := range cmd.Env {
					key, _, _ := strings.Cut(entry, "=")
					if !strings.EqualFold(key, "HOME") && key != "USERPROFILE" {
						filtered = append(filtered, entry)
					}
				}
				cmd.Env = filtered
			}
			output, runErr := cmd.CombinedOutput()
			if _, err := os.Stat(projectMarker); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("project alias executed with %s HOME: %v", name, err)
			}
			if filepath.IsAbs(home) {
				if runErr != nil {
					t.Fatalf("trusted absolute HOME failed: %v: %s", runErr, output)
				}
				if _, err := os.Stat(trustedMarker); err != nil {
					t.Fatalf("trusted HOME alias did not execute: %v", err)
				}
			} else if exitErr, ok := errors.AsType[*exec.ExitError](runErr); !ok || exitErr.ExitCode() != 1 {
				t.Errorf("untrusted HOME exit = %v, want 1; output: %s", runErr, output)
			}
		})
	}
}

func writeHomeStorageAlias(t *testing.T, dir, marker string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	command := ": > '" + strings.ReplaceAll(marker, "'", "'\"'\"'") + "'"
	if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte(fmt.Sprintf("storagefixture: %q\n", command)), 0o600); err != nil {
		t.Fatal(err)
	}
}
