//go:build e2e

package deploy_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	prefixEnv = "PREFIX"
)

func TestShellUninstallerKeepsConfigByDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	prefix := filepath.Join(root, "bin")
	home := filepath.Join(root, "home")
	config := filepath.Join(home, ".zev")
	mustMkdir(t, prefix, config)
	mustWrite(t, filepath.Join(prefix, "zev"), "binary")
	mustWrite(t, filepath.Join(config, "config"), "LLM_PROVIDER=ollama\n")

	runUninstallerScript(t, filepath.Join(repoRoot(t), "deploy", "uninstall.sh"), map[string]string{
		"HOME":    home,
		prefixEnv: prefix,
	})
	assertMissing(t, filepath.Join(prefix, "zev"))
	assertExists(t, filepath.Join(config, "config"))
}

func TestShellUninstallerRemovesConfigWithExplicitOptIn(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	prefix := filepath.Join(root, "bin")
	home := filepath.Join(root, "home")
	config := filepath.Join(home, ".zev")
	mustMkdir(t, prefix, config)
	mustWrite(t, filepath.Join(prefix, "zev"), "binary")
	mustWrite(t, filepath.Join(config, "history"), "history")

	runUninstallerScript(t, filepath.Join(repoRoot(t), "deploy", "uninstall.sh"), map[string]string{
		"HOME":              home,
		prefixEnv:           prefix,
		"ZEV_REMOVE_CONFIG": "1",
	})
	assertMissing(t, filepath.Join(prefix, "zev"))
	assertMissing(t, config)
}

func TestPowerShellUninstallerRemovesConfigWithExplicitOptIn(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("PowerShell executable fixture uses pwsh on the host")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skipf("pwsh is unavailable: %v", err)
	}
	root := t.TempDir()
	prefix := filepath.Join(root, "programs", "zev")
	home := filepath.Join(root, "home")
	config := filepath.Join(home, ".zev")
	mustMkdir(t, prefix, config)
	mustWrite(t, filepath.Join(prefix, "zev.exe"), "binary")
	mustWrite(t, filepath.Join(config, "config"), "LLM_PROVIDER=ollama\n")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, pwsh, "-NoLogo", "-NoProfile", "-File", filepath.Join(repoRoot(t), "deploy", "uninstall.ps1"), "-Prefix", prefix) //nolint:gosec // Runs the repository uninstaller against an isolated t.TempDir fixture.
	cmd.Env = append(os.Environ(), "HOME="+home, "ZEV_REMOVE_CONFIG=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell uninstaller failed: %v\n%s", err, output)
	}
	assertMissing(t, filepath.Join(prefix, "zev.exe"))
	assertMissing(t, config)
}

func runUninstallerScript(t *testing.T, script string, values map[string]string) {
	t.Helper()
	requireBash(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "bash", script) //nolint:gosec // Runs a repository-owned script with isolated fixture paths.
	cmd.Env = makeDeploymentEnv(values)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstaller failed: %v\n%s", err, output)
	}
}

func makeDeploymentEnv(values map[string]string) []string {
	blocked := map[string]bool{"ZEV_REMOVE_CONFIG": true}
	for key := range values {
		blocked[key] = true
	}
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if blocked[name] {
			continue
		}
		env = append(env, entry)
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still exists or cannot be checked: %v", path, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s is missing: %v", path, err)
	}
}
