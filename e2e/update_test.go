//go:build e2e

package e2e_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateMigrateOnlyRunsAllLocalStores(t *testing.T) {
	t.Parallel()
	home := seedLegacyHistory(t)
	dir := filepath.Join(home, ".zev")
	files := map[string]string{
		"config":               "LLM_PROVIDER=ollama\nOLLAMA_MODEL=preserved-model\n",
		"aliases":              "show: echo {{.arg1}}\n",
		"scripts":              "safe:\n  script: safe.sh\n  description: preserved description\n",
		"script_files/safe.sh": "#!/bin/bash\nprintf preserved-body\n",
	}
	if err := os.MkdirAll(filepath.Join(dir, "script_files"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var firstConfig []byte
	for attempt := range 2 {
		cmd := cliCommand(t, home, "update", "--migrate-only")
		var diagnostics bytes.Buffer
		cmd.Stderr = &diagnostics
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("migrate-only: %v\n%s\n%s", err, output, diagnostics.String())
		}
		if !strings.Contains(string(output), "All Zev data migrations completed") {
			t.Fatalf("unexpected success: %s", output)
		}
		data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
		if err != nil || !bytes.Contains(data, []byte("version: 1\n")) || !bytes.Contains(data, []byte("preserved-model")) {
			t.Fatalf("version or model missing after migration: %v", err)
		}
		if attempt == 0 {
			firstConfig = data
		} else if !bytes.Equal(data, firstConfig) {
			t.Fatal("retry changed the current config")
		}
		for _, name := range []string{"configuration", "aliases", "scripts", "history"} {
			if !strings.Contains(diagnostics.String(), name) {
				t.Errorf("missing %s status: %s", name, diagnostics.String())
			}
		}
		for _, pattern := range []string{".aliases-migration/*.archive", ".scripts-migration/*.archive", "history.migrated.*.jsonl", "config*.bak"} {
			matches, err := filepath.Glob(filepath.Join(dir, pattern))
			if err != nil || len(matches) != 0 {
				t.Fatalf("migration left a backup after run %d: %v (%v)", attempt+1, matches, err)
			}
		}
	}
	for _, name := range []string{"aliases", "scripts"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !strings.HasPrefix(string(data), "version: 2\n") {
			t.Fatalf("%s not migrated: %q, %v", name, data, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "script_files", "safe.sh"))
	if err != nil || string(body) != files["script_files/safe.sh"] {
		t.Fatalf("script body changed: %q, %v", body, err)
	}
	views := exportHistory(t, home)
	if len(views) != 1 {
		t.Fatalf("history migrated %d times; want 1", len(views))
	}
}

func TestUpdateMigrateOnlyFailsWithoutHidingOtherMigrations(t *testing.T) {
	t.Parallel()
	home := seedLegacyHistory(t)
	dir := filepath.Join(home, ".zev")
	if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte("bad: [invalid-template]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := cliCommand(t, home, "update", "--migrate-only")
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	output, err := cmd.Output()
	if err == nil || len(output) != 0 {
		t.Fatalf("failed migration reported success: %q, %v", output, err)
	}
	if !strings.Contains(diagnostics.String(), "aliases") || !strings.Contains(diagnostics.String(), "history") {
		t.Fatalf("missing failure or subsequent migration: %s", diagnostics.String())
	}
	views := exportHistory(t, home)
	if len(views) != 1 {
		t.Fatalf("independent history migration did not complete: count=%d", len(views))
	}
}

func TestUpdateDoesNotAdoptUnreleasedConfig(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// This named-profile YAML never shipped. Only the released dotenv file
	// has a supported path from an unversioned configuration to v1.
	original := []byte("active_provider: local\nproviders: {local: {type: ollama}}\n")
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cliCommand(t, home, "update", "--migrate-only").CombinedOutput(); err == nil {
		t.Fatal("adopted unreleased YAML as a released schema")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("unsupported file changed: %v", err)
	}
}

func TestUpdateProtocolIsLocalAndLanguageIndependent(t *testing.T) {
	t.Parallel()
	for _, language := range []string{"en", "ru"} {
		home := t.TempDir()
		writeFixture(t, home, cliFixture{language: language})
		cmd := cliCommand(t, home, "version", "--update-protocol")
		output, err := cmd.CombinedOutput()
		if err != nil || string(output) != "zev-update-v1\n" {
			t.Fatalf("protocol %s: %q, %v", language, output, err)
		}
	}
}
