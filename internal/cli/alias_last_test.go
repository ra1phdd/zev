package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/metadata"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	historyrepo "github.com/ra1phdd/zev/internal/entities/history/repository"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
)

const (
	lastAliasName          = "recent"
	aliasHistoryFixture    = "fixture"
	lastAliasCommand       = "printf hello"
	managedDescriptionFlag = "--description"
)

func TestAliasLast(t *testing.T) {
	// HOME selects real composition-root storage; each subtest uses an isolated home.
	for _, tc := range []struct {
		name      string
		seed      bool
		existing  string
		wantError string
	}{
		{name: "saves selected command in existing YAML", seed: true, existing: "version: 2\naliases:\n  other:\n    template: echo untouched\n"},
		{name: "empty history", wantError: "no usable selected command"},
		{name: "name conflict", seed: true, existing: "version: 2\naliases:\n  recent:\n    template: echo original\n", wantError: "already exists"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			// This operation must not load even an invalid provider/embedding config.
			dir := filepath.Join(home, ".zev")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("version: 1\nhistory_semantic_enabled: true\nhistory_embedding_provider: invalid\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "aliases")
			if tc.existing != "" {
				if err := os.WriteFile(path, []byte(tc.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.seed {
				store, err := openHistoryStore(t.Context(), filepath.Join(dir, "history.db"))
				if err != nil {
					t.Fatal(err)
				}
				seedAliasHistory(t, store, "good", lastAliasCommand, "completed", "printed", time.Unix(100, 0))
				seedAliasHistory(t, store, "cancelled", "echo cancelled", "cancelled", "printed", time.Unix(200, 0))
				seedAliasHistory(t, store, "failed", "echo failed", "failed", "failed", time.Unix(300, 0))
				seedAliasHistory(t, store, "unselected", "echo unselected", "completed", "", time.Unix(400, 0))
				seedAliasHistory(t, store, "pending", "echo pending", "completed", "selected", time.Unix(500, 0))
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			cmd := newAliasCmd()
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs([]string{"last", lastAliasName})
			err := cmd.ExecuteContext(t.Context())
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error=%v, want %q", err, tc.wantError)
				}
				if out.Len() != 0 {
					t.Fatalf("success output on failure: %q", out.String())
				}
				if tc.existing != "" {
					data, readErr := os.ReadFile(path) //nolint:gosec // Read only the fixture under the isolated HOME.
					if readErr != nil {
						t.Fatal(readErr)
					}
					if string(data) != tc.existing {
						t.Fatalf("aliases overwritten: %q", data)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), lastAliasCommand) || !strings.Contains(out.String(), "zev "+lastAliasName) {
				t.Fatalf("confirmation=%q", out.String())
			}
			data, err := os.ReadFile(path) //nolint:gosec // Read only the fixture under the isolated HOME.
			if err != nil {
				t.Fatal(err)
			}
			values, err := metadata.DecodeAliases(data)
			if err != nil {
				t.Fatalf("decode aliases: %v", err)
			}
			if values[lastAliasName].Template != lastAliasCommand || values["other"].Template != "echo untouched" {
				t.Fatalf("YAML=%q", data)
			}
			resolved, err := aliasservice.New(aliasrepo.New(dir)).Resolve(lastAliasName, nil)
			if err != nil || resolved != lastAliasCommand {
				t.Fatalf("resolved=%q err=%v", resolved, err)
			}
		})
	}
}

func TestAliasLastSupportsManualMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := openHistoryStore(t.Context(), filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	seedAliasHistory(t, store, "last-metadata", "printf {{.arg1}}", "completed", "printed", time.Unix(100, 0))
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := newAliasCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"last", lastAliasName, managedDescriptionFlag, "manual description", "--arguments-json", `[{"name":"value","description":"a value","required":false}]`})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "aliases")) //nolint:gosec // Isolated fixture store.
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := metadata.DecodeAliases(data)
	if err != nil {
		t.Fatal(err)
	}
	alias := aliases[lastAliasName]
	if alias.Template != "printf {{.arg1}}" || alias.Description != "manual description" || len(alias.Arguments) != 1 || alias.Arguments[0].Required {
		t.Fatalf("saved alias = %#v", alias)
	}
}

func TestAliasAddPersistsMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := newAliasCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{managedOpAdd, "show", "echo {{.arg1}}", "--description", "display a value", "--arguments-json", `[{"name":"value","description":"text"}]`})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "aliases")) //nolint:gosec // Read only the isolated test store.
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := metadata.DecodeAliases(data)
	if err != nil {
		t.Fatalf("decode aliases: %v", err)
	}
	alias := aliases["show"]
	if alias.Description != "display a value" || len(alias.Arguments) != 1 || !alias.Arguments[0].Required {
		t.Fatalf("alias metadata=%#v", alias)
	}
}

func seedAliasHistory(t *testing.T, s *historyrepo.Store, id, command, status, outcome string, at time.Time) {
	t.Helper()
	if _, err := s.StartConversation(t.Context(), historyservice.StartConversationInput{
		Conversation:   historyservice.Conversation{ID: id, CreatedAt: at},
		InitialMessage: historyservice.Message{ID: id + "-user", Role: "user", Kind: "initial_prompt", Content: aliasHistoryFixture},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordResponse(t.Context(), historyservice.RecordResponseInput{
		Message: historyservice.Message{ID: id + "-assistant", Role: queryAssistantRole, Kind: "response", Content: aliasHistoryFixture},
		Response: historyservice.Response{ID: id + "-response", ConversationID: id, PayloadJSON: `{}`, IsValid: true, Commands: []historyservice.Command{
			{ID: id + "-unchosen", Command: "echo wrong alternative"}, {ID: id + "-command", Command: command},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if outcome != "" {
		if err := s.RecordSelection(t.Context(), historyservice.RecordSelectionInput{ConversationID: id, CommandID: id + "-command", SelectedAt: at, DeliveryOutcome: outcome}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CompleteConversation(t.Context(), historyservice.CompleteConversationInput{ConversationID: id, Status: status, CompletedAt: at.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
}
