//go:build e2e

package e2e_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"gopkg.in/yaml.v3"
)

var cliBinary string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	root := filepath.Dir(cwd)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "find repository root from e2e directory: %v\n", err)
		return 1
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Each test invocation owns only its artifact directory, even in concurrent runs.
	artifact, err := os.MkdirTemp(bin, "zev-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(artifact); err != nil {
			fmt.Fprintf(os.Stderr, "remove e2e artifact: %v\n", err)
		}
	}()
	name := "zev"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cliBinary = filepath.Join(artifact, name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", cliBinary, "./cmd/zev") //nolint:gosec // Builds a fixed package into this invocation's owned artifact directory.
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build e2e CLI: %v\n", err)
		return 1
	}
	return m.Run()
}

func isolatedEnv(t *testing.T, home string) []string {
	t.Helper()
	tmp := t.TempDir()
	env := []string{
		"HOME=" + home, "USERPROFILE=" + home, "home=" + home,
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"LANG=en", "LC_ALL=C", "NO_COLOR=1",
	}
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func cliCommand(t *testing.T, home string, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, cliBinary, args...) //nolint:gosec // The executable is built by TestMain and arguments are controlled test fixtures.
	cmd.Dir = home
	cmd.Env = isolatedEnv(t, home)
	return cmd
}

type cliFixture struct {
	baseURL, key, language, toolApproval, securityMode string
	history                                            bool
}

func writeFixture(t *testing.T, home string, f cliFixture) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.ActiveProvider = "fixture"
	// A command without a provider fixture must never contact an external service.
	cfg.Providers = map[string]config.ProviderConfig{"fixture": {Type: "openai", Model: fixtureModel, BaseURL: cmp.Or(f.baseURL, "http://127.0.0.1:1"), APIKey: f.key}}
	cfg.UILanguage = cmp.Or(f.language, "auto")
	cfg.ToolApprovalMode = cmp.Or(f.toolApproval, "manual")
	if f.securityMode != "" {
		cfg.Security.Mode = f.securityMode
	}
	cfg.HistoryEnabled = f.history
	cfg.ContextEnabled = false
	cfg.ToolsEnabled = false
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	writeFixture(t, home, cliFixture{securityMode: config.SecurityModeBypass})
	return home
}

func setCLIEnv(cmd *exec.Cmd, values map[string]string) {
	for key, value := range values {
		filtered := cmd.Env[:0]
		for _, entry := range cmd.Env {
			name, _, _ := strings.Cut(entry, "=")
			if !strings.EqualFold(name, key) {
				filtered = append(filtered, entry)
			}
		}
		cmd.Env = filtered
		cmd.Env = append(cmd.Env, key+"="+value)
	}
}

func reapCLI(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	t.Cleanup(func() {
		if cmd.ProcessState != nil {
			return
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill CLI: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			if exitErr, exited := errors.AsType[*exec.ExitError](err); !exited || exitErr == nil {
				t.Errorf("reap CLI: %v", err)
			}
		}
	})
}
