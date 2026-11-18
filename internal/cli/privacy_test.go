package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const privacyConfig = "persisted config ignores ambient overrides"

// The CLI uses global flags and process-wide HOME.
func TestQueryPrivacyControls(t *testing.T) {
	for _, source := range []string{privacyConfig, "flags", "query subcommand"} {
		t.Run(source, func(t *testing.T) {
			verbosity, execute, inject := verbosityLevel, executeFlag, injectFlag
			noHistory, noContext := noHistoryFlag, noContextFlag
			t.Cleanup(func() {
				verbosityLevel, executeFlag, injectFlag = verbosity, execute, inject
				noHistoryFlag, noContextFlag = noHistory, noContext
			})
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("USER", "private-user-marker")
			shell := filepath.Join(home, "private-shell-marker")
			if err := os.Symlink("/bin/sh", shell); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SHELL", shell)
			var payload struct {
				Messages []struct{ Role, Content string }
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				_, err := io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"is_valid\":true,\"commands\":[{\"command\":\"printf private-command-marker\",\"short_explanation\":\"test\"}]}"}}]}`)
				if err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			cfg := openAITestConfig(server.URL, "private-key-marker")
			cfg.HTTPHeaders = map[string]string{"X-Private": "private-header-marker"}
			cfg.HistoryEnabled = source != privacyConfig
			cfg.ContextEnabled = source != privacyConfig
			writeCLIConfig(t, home, cfg)
			if source == privacyConfig {
				t.Setenv("LLM_PROVIDER", "unsupported-ambient-provider")
				t.Setenv("HISTORY_ENABLED", "true")
				t.Setenv("CONTEXT_ENABLED", "true")
			}
			var out, diagnostics bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			args := []string{"--execute", "private-prompt-marker"}
			if source != privacyConfig {
				args = append([]string{"--no-history", "--no-context"}, args...)
			}
			if source == "query subcommand" {
				cmd = newQueryCmd()
				cmd.SetOut(&out)
				cmd.SetErr(&diagnostics)
				cmd.SetArgs(args)
				if err := cmd.ExecuteContext(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else if err := runQuery(cmd, args); err != nil {
				t.Fatal(err)
			}
			if len(payload.Messages) != 2 {
				t.Fatalf("messages = %d, want system and user", len(payload.Messages))
			}
			pwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"private-user-marker", "private-shell-marker", home, pwd} {
				if strings.Contains(payload.Messages[0].Content, forbidden) {
					t.Errorf("disabled context leaked %q", forbidden)
				}
			}
			for _, message := range payload.Messages {
				for _, secret := range []string{"private-key-marker", "private-header-marker"} {
					if strings.Contains(message.Content, secret) {
						t.Error("provider payload disclosed configured credentials")
					}
				}
			}
			if payload.Messages[1].Content != "private-prompt-marker" {
				t.Errorf("user prompt changed: %q", payload.Messages[1].Content)
			}
			for _, name := range []string{historyRootCommand, "history.db", "history.db-wal", "history.db-shm", "history.lock"} {
				if _, err := os.Stat(filepath.Join(home, ".zev", name)); !os.IsNotExist(err) {
					t.Fatalf("disabled history created %s: %v", name, err)
				}
			}
			if diagnostics.Len() != 0 {
				t.Errorf("unexpected diagnostics: %s", &diagnostics)
			}
		})
	}
}

func TestQuerySessionCancellationBeforeUI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	err := runSetupHeaders(cmd, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled header setup error = %v, want context.Canceled", err)
	}
}
