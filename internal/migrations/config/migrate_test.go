package config

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	current "github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/migrations/filetxn"
)

const (
	openAI    = "openai"
	canonical = "config.yaml"
	// v1.0.0 Save used the OpenAI slot for OpenCode and retained inactive providers.
	released = `LLM_PROVIDER=opencode
HTTP_HEADERS=x-opencode-session:session,User-Agent:code
OPENAI_API_KEY=fixture
OPENAI_MODEL=code
OPENAI_BASE_URL=https://example.invalid
GEMINI_API_KEY=other-fixture
GEMINI_MODEL=gem
GEMINI_BASE_URL=https://gemini.invalid
ANTHROPIC_MODEL=claude
OLLAMA_MODEL=local
COMMAND_EXECUTION_MODE=execute
SCRIPT_EDITOR=vim
`
)

func put(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateReleasedConfigurationAndConcurrentUpdates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GEMINI_MODEL", "ignore-me")
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "config"), released)
	if lang, err := current.LoadUILanguage(); err != nil || lang != "auto" {
		t.Fatalf("language before migration: %q, %v", lang, err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatal("language lookup modified storage")
	}
	before, err := current.Load()
	if err != nil || before.ActiveProvider != openAI {
		t.Fatalf("core did not ignore dotenv: %v", err)
	}
	const n = 12
	errs := make(chan error, n)
	for range n {
		go func() {
			if err := Migrate(t.Context(), dir); err != nil {
				errs <- err
				return
			}
			errs <- current.Update(func(c *current.Config) error { c.HistoryEmbeddingMaxBytes++; return nil })
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	got, err := current.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.ProviderNames(), []string{"anthropic", "gemini", "ollama", openAI, "opencode"}) || got.Providers["gemini"].Model != "gem" || got.Providers["gemini"].APIKey != "other-fixture" || got.ActiveProvider != "opencode" || got.OpenAIAPIKey != "fixture" || got.CommandExecutionMode != "execute" || got.ScriptEditor != "vim" || got.HistoryEmbeddingMaxBytes != current.DefaultConfig().HistoryEmbeddingMaxBytes+n {
		t.Fatal("migration lost settings or concurrent update")
	}
	if got.EffectiveHTTPHeaders()["X-Opencode-Session"] != "session" {
		t.Fatal("missing OpenCode session")
	}
	got.ActiveProvider = openAI
	if got.EffectiveHTTPHeaders()["X-Opencode-Session"] != "" {
		t.Fatal("session leaked across profiles")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !slices.Contains([]string{canonical, "config.lock", "config.yaml.lock"}, entry.Name()) {
			t.Fatalf("unexpected artifact: %s", entry.Name())
		}
	}
}

func TestMigrateRejectsInvalidSource(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"LLM_PROVIDER=secret-bad", "HTTP_HEADERS=secret-bad", "ZEV_LANG=secret-bad", "HISTORY_ENABLED=false"} {
		dir := t.TempDir()
		src := filepath.Join(dir, "config")
		put(t, src, source)
		if err := Migrate(t.Context(), dir); err == nil || strings.Contains(err.Error(), "secret-bad") {
			t.Fatalf("missing or unsafe error: %v", err)
		}
		if data, err := os.ReadFile(filepath.Clean(src)); err != nil || string(data) != source {
			t.Fatal("source lost")
		}
		if _, err := os.Stat(filepath.Join(dir, canonical)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid output published")
		}
	}
}

func TestMigrateExistingYAML(t *testing.T) {
	t.Parallel()
	const fields = "active_provider: work\nproviders: {work: {type: openai}}\n"
	for _, tc := range []struct {
		version string
		valid   bool
	}{{"", false}, {"version: 99\n", false}, {"version: 1 # keep formatting\n", true}} {
		dir := t.TempDir()
		dest := filepath.Join(dir, canonical)
		source := tc.version + fields
		put(t, dest, source)
		if err := Migrate(t.Context(), dir); (err == nil) != tc.valid {
			t.Fatalf("unexpected version outcome: %v", err)
		}
		src := filepath.Join(dir, "config")
		put(t, src, released)
		err := Migrate(t.Context(), dir)
		if err == nil || tc.valid && !errors.Is(err, filetxn.ErrConflict) {
			t.Fatalf("expected conflict: %v", err)
		}
		if data, err := os.ReadFile(filepath.Clean(src)); err != nil || string(data) != released {
			t.Fatal("conflict changed source")
		}
		if data, err := os.ReadFile(filepath.Clean(dest)); err != nil || string(data) != source {
			t.Fatal("changed existing YAML")
		}
	}
}

func TestMigrateRecoversV1Journal(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"prepared", "published", "source removed"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src := filepath.Join(dir, "config")
			dest := filepath.Join(dir, canonical)
			stage := dest + ".migration.stage"
			put(t, src, released)
			// Block staging so the production writer prepares its actual journal.
			put(t, stage, "occupied")
			if err := Migrate(t.Context(), dir); !errors.Is(err, filetxn.ErrConflict) {
				t.Fatalf("prepare: %v", err)
			}
			if err := os.Remove(stage); err != nil {
				t.Fatal(err)
			}
			output, err := dotenvToV1([]byte(released))
			if err != nil {
				t.Fatal(err)
			}
			if phase != "prepared" {
				put(t, dest, string(output))
			}
			if phase == "source removed" {
				if err := os.Remove(src); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := Migrate(t.Context(), dir); err != nil {
					t.Fatal(err)
				}
			}
			if data, err := os.ReadFile(filepath.Clean(dest)); err != nil || !bytes.Equal(data, output) || !bytes.HasPrefix(data, []byte("version: 1\n")) {
				t.Fatal("unexpected recovered output")
			}
			for _, path := range []string{src, stage, dest + ".migration"} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected migration artifact: %v", err)
				}
			}
		})
	}
}

func TestMigrationWaitsForBothLocks(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"config", canonical} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			put(t, filepath.Join(dir, "config"), released)
			err := filestore.WithLock(filepath.Join(dir, name), func() error {
				ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
				defer cancel()
				return Migrate(ctx, dir)
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lock not honored: %v", err)
			}
		})
	}
}
