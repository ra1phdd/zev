//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	englishPrompt   = "Show the largest files in this directory"
	russianLocale   = "ru_RU.UTF-8"
	fixtureModel    = "fixture-model"
	fixtureOnly     = "fixture-only"
	russianProgress = "Получение вариантов команд"
	englishProgress = "Fetching suggestions"
)

func TestQueryLanguageAndReadOnlyPipeline(t *testing.T) {
	t.Parallel()
	const command = "du -ah . | sort -hr | head -n 20"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		content := `{"is_valid":true,"commands":[{"command":"` + command + `","short_explanation":"fixture explanation","is_dangerous":false}]}`
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}}}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	for _, tc := range []struct{ name, prompt, configured, override, system, want string }{
		{name: "Russian auto", prompt: "Покажи самые большие файлы в текущей директории", system: "en", want: russianProgress},
		{name: "English auto ignores Russian system", prompt: englishPrompt, system: russianLocale, want: englishProgress},
		{name: "Bulgarian falls back", prompt: "Покажи най-големите файлове в тази директория", system: russianLocale, want: englishProgress},
		{name: "fixed English", prompt: "Покажи самые большие файлы в текущей директории", configured: "en", system: russianLocale, want: englishProgress},
		{name: "fixed Russian", prompt: englishPrompt, configured: "ru", system: "en", want: russianProgress},
		{name: "ambient ZEV_LANG is ignored", prompt: "Покажи самые большие файлы в текущей директории", configured: "en", override: "ru", system: russianLocale, want: englishProgress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			writeFixture(t, home, cliFixture{baseURL: server.URL, key: fixtureOnly, language: tc.configured})
			cmd := cliCommand(t, home, "query", "--json", "--no-history", tc.prompt)
			setCLIEnv(cmd, map[string]string{"LANG": tc.system, "LC_ALL": tc.system})
			if tc.override != "" {
				setCLIEnv(cmd, map[string]string{"ZEV_LANG": tc.override})
			}
			var out, diagnostic bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &diagnostic
			if err := cmd.Run(); err != nil {
				t.Fatalf("query failed: %v, %s", err, diagnostic.String())
			}
			if !strings.Contains(diagnostic.String(), tc.want) {
				t.Fatalf("UI=%q, want %q", diagnostic.String(), tc.want)
			}
			var got struct {
				SchemaVersion int    `json:"schema_version"` //nolint:tagliatelle // Existing machine contract uses snake_case.
				Status        string `json:"status"`
				Commands      []struct {
					Command   string `json:"command"`
					Dangerous bool   `json:"is_dangerous"` //nolint:tagliatelle // Existing machine contract uses snake_case.
				} `json:"commands"`
			}
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.SchemaVersion != 1 || got.Status != "ok" || len(got.Commands) != 1 || got.Commands[0].Command != command || got.Commands[0].Dangerous {
				t.Fatalf("machine contract changed: %s", out.String())
			}
			if strings.Contains(diagnostic.String(), "⚠") || strings.Contains(diagnostic.String(), "Warning:") {
				t.Fatalf("safe pipeline warned: %s", diagnostic.String())
			}
		})
	}
}

func TestConfiguredLanguageAppliesToHelp(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFixture(t, home, cliFixture{language: "ru"})
	cmd := cliCommand(t, home, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help: %v, %s", err, output)
	}
	if !bytes.Contains(output, []byte("Использование:")) {
		t.Fatalf("help not Russian: %s", output)
	}
}
