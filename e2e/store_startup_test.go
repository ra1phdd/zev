//go:build e2e

package e2e_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	startupAliases = "aliases"
	startupScripts = "scripts"
	startupList    = "list"
	startupVersion = "version"
)

// v1.0.0 wrote dotenv config, name-to-template aliases and name-to-record scripts.
func TestReleasedStoresUpgradeOnUse(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(filepath.Join(dir, "script_files"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"config":                "LLM_PROVIDER=ollama\nOLLAMA_MODEL=fixture\nOLLAMA_BASE_URL=http://127.0.0.1:1\nCOMMAND_EXECUTION_MODE=execute\n",
		startupAliases:          "greet: echo {{.arg1}}\n",
		startupScripts:          "hello:\n  script: hello.sh\n  description: Released script\n",
		"script_files/hello.sh": "printf 'hello\\n'\n",
		"history":               legacyLine,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	unchanged := func(names ...string) {
		t.Helper()
		for _, name := range names {
			b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // Only reads the test's released fixtures.
			if err != nil || string(b) != files[name] {
				t.Fatalf("unexpected change to %s: %v", name, err)
			}
		}
	}
	for _, args := range [][]string{{managedE2EHelpFlag}, {startupVersion}, {"doctor", managedE2EJSONFlag}} {
		cmd := cliCommand(t, home, args...)
		out, err := cmd.Output()
		// Doctor may report unconfigured providers or missing SQLite.
		if len(out) == 0 || err != nil && args[0] != "doctor" {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
		unchanged("config", "history", startupAliases, startupScripts)
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != len(files) {
			t.Fatalf("%v created storage files: %v (%v)", args, entries, err)
		}
	}
	run := func(want string, args ...string) {
		t.Helper()
		cmd := cliCommand(t, home, args...)
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), want) {
			t.Fatalf("%v: want %q, got %q (%v)", args, want, out, err)
		}
	}
	run("echo", "query", managedE2EPrintFlag, "greet", "world")
	unchanged("config", "history", startupScripts)
	run("hello", "hello")
	unchanged("config", "history", "script_files/hello.sh")
	run("execute", "setup", "mode")
	unchanged("history")
	if _, err := os.Lstat(filepath.Join(dir, "config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("setup did not consume released config: %v", err)
	}
	for _, name := range []string{"config.yaml", startupAliases, startupScripts} {
		b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // Only reads the upgraded fixtures.
		if err != nil || !bytes.Contains(b, []byte("version:")) {
			t.Fatalf("%s was not upgraded: %v", name, err)
		}
	}
}

func TestSetupStopsBeforePromptOnUnsupportedConfig(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	const source = "LLM_PROVIDER=ollama\nUNKNOWN_SETTING=private-marker\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := cliCommand(t, home, "setup")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if out, err := cmd.Output(); err == nil || len(out) != 0 || !strings.Contains(stderr.String(), "migrate configuration") || strings.Contains(stderr.String(), "private-marker") {
		t.Fatalf("setup did not fail safely before prompting: stdout=%q stderr=%q err=%v", out, stderr.String(), err)
	}
	b, err := os.ReadFile(path) //nolint:gosec // Reads only the isolated fixture.
	if err != nil || string(b) != source {
		t.Fatalf("source changed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported config was published: %v", err)
	}
}

func TestInvalidMachinePrefixDoesNotTouchStorage(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, startupAliases), []byte("broken: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := cliCommand(t, home, managedE2EJSONFlag, managedE2EPrintFlag, "greet").Output()
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exitErr.ExitCode() != 2 || len(out) != 0 {
		t.Fatalf("bad machine flags: stdout=%q err=%v", out, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != startupAliases {
		t.Fatalf("bad flags touched storage: %v (%v)", entries, err)
	}
}

func TestReadOnlyCLILeavesMissingZevDirAbsent(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{startupVersion}, {managedE2EHelpFlag}, {"doctor", managedE2EJSONFlag}, {"unknown", managedE2EHelpFlag}, {"alias", startupList}, {"script", startupList}, {"provider", startupList}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			cmd := cliCommand(t, home, args...)
			if _, err := cmd.Output(); err != nil && !slices.Contains([]string{"doctor", "unknown"}, args[0]) {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(home, ".zev")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%v created .zev: %v", args, err)
			}
		})
	}
}
