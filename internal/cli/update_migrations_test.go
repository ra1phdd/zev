package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/metadata"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestUpdateMigrationsConvertAllStoresAndRetry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(filepath.Join(dir, "script_files"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]string{
		"config":               "LLM_PROVIDER=gemini\nGEMINI_MODEL=migration-fixture-model\n",
		"aliases":              "show: echo {{.arg1}}\n",
		"scripts":              "safe:\n  script: safe.sh\n  description: preserved description\n",
		"script_files/safe.sh": "#!/bin/bash\nprintf preserved-body\n",
		"history":              `{"timestamp":"2024-03-02T01:02:03Z","prompt":"find archival files","response":{"is_valid":true,"commands":[{"command":"find .","short_explanation":"search files"}]},"provider":"legacy-provider","model":"legacy-model"}` + "\n",
	}
	for name, data := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	for range 2 {
		if err := runUpdateMigrations(t.Context(), &output); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("configuration was not migrated: %v", err)
	}
	cfg, err := config.Load()
	if err != nil || cfg.ActiveProvider != "gemini" || cfg.Providers[cfg.ActiveProvider].Model != "migration-fixture-model" {
		t.Fatalf("configuration migration lost the provider model: %v", err)
	}
	aliasesData, err := os.ReadFile(filepath.Join(dir, "aliases")) //nolint:gosec // Reads the isolated migration fixture created in this test.
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := metadata.DecodeAliases(aliasesData)
	if err != nil || aliases["show"].Template != "echo {{.arg1}}" {
		t.Fatalf("aliases not migrated: %#v, error=%v", aliases, err)
	}
	scriptsData, err := os.ReadFile(filepath.Join(dir, "scripts")) //nolint:gosec // Reads the isolated migration fixture created in this test.
	if err != nil {
		t.Fatal(err)
	}
	scripts, err := metadata.DecodeScripts(scriptsData)
	if err != nil || scripts["safe"].Description != "preserved description" || scripts["safe"].RecordID == "" {
		t.Fatalf("scripts not migrated: %#v, error=%v", scripts, err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "script_files", "safe.sh")) //nolint:gosec // Reads the isolated migration fixture created in this test.
	if err != nil || string(body) != fixtures["script_files/safe.sh"] {
		t.Fatalf("body changed: %q, %v", body, err)
	}
	store, err := openHistoryStore(t.Context(), filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	conversations, listErr := store.ListConversations(t.Context(), historyservice.ListRequest{Limit: 10})
	closeErr := store.Close()
	if err := errors.Join(listErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].InitialPrompt != "find archival files" {
		t.Fatalf("history not migrated exactly once: %#v", conversations)
	}
	for _, name := range []string{"configuration", "aliases", "scripts", "history"} {
		if !strings.Contains(output.String(), name) {
			t.Errorf("missing migration status for %s: %s", name, output.String())
		}
	}
}

func TestUpdateMigrationsMissingDirectoryDoesNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := runUpdateMigrations(t.Context(), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zev")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("migration created state for new user: %v", err)
	}
}

func TestUpdateMigrationsRejectSymlinkDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(home, ".zev")); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	if err := runUpdateMigrations(t.Context(), &bytes.Buffer{}); err == nil {
		t.Fatal("accepted a symlinked application directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatalf("symlink target was changed: entries=%d, error=%v", len(entries), err)
	}
}
