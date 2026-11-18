//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const legacyLine = `{"timestamp":"2024-03-02T01:02:03Z","prompt":"find archival files","provider":"legacy-provider","model":"legacy-model","response":{"is_valid":true,"commands":[{"command":"find . -type f","short_explanation":"archive explanation"}]},"selected_command":"find . -type f","selected_index":0}` + "\n"

func TestHistoryMigrationConcurrentCLIQueries(t *testing.T) {
	t.Parallel()
	home := seedLegacyHistory(t)
	requests := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read provider request: %v", err)
			return
		}
		requests <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"is_valid\":true,\"commands\":[{\"command\":\"echo migration-fixture\",\"short_explanation\":\"fixture\"}]}"}}]}`); err != nil {
			t.Errorf("write provider response: %v", err)
		}
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	writeFixture(t, home, cliFixture{baseURL: server.URL, key: "migration-secret-marker", history: true})
	commands := make([]*exec.Cmd, 2)
	outputs := make([]bytes.Buffer, 2)
	diagnostics := make([]bytes.Buffer, 2)
	for i, prompt := range []string{"concurrent writer a", "concurrent writer b"} {
		cmd := cliCommand(t, home, "--json", "--no-context", prompt)
		cmd.Stdout, cmd.Stderr = &outputs[i], &diagnostics[i]
		commands[i] = cmd
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		reapCLI(t, cmd)
	}
	// Both processes must reach the provider before either query may finish.
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	for range commands {
		select {
		case <-requests:
		case <-ctx.Done():
			t.Fatal("concurrent CLI queries did not reach the provider")
		}
	}
	close(release)
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("query %d: %v; stderr: %s", i, err, diagnostics[i].String())
		}
		var result struct{ Status string }
		if err := json.Unmarshal(outputs[i].Bytes(), &result); err != nil || result.Status != "ok" {
			t.Fatalf("query %d output: %s; error: %v", i, outputs[i].String(), err)
		}
		if strings.Contains(outputs[i].String()+diagnostics[i].String(), "migration-secret-marker") {
			t.Fatal("CLI output leaked provider credential")
		}
	}
	views := exportHistory(t, home)
	if len(views) != 3 {
		t.Fatalf("exported conversations = %d, want one legacy and two queries; query stderr: [%s] [%s]", len(views), diagnostics[0].String(), diagnostics[1].String())
	}
	prompts := make(map[string]int)
	ids := make(map[string]bool)
	for _, view := range views {
		if view.Conversation.ID == "" || ids[view.Conversation.ID] {
			t.Fatalf("empty or duplicate conversation ID: %q", view.Conversation.ID)
		}
		ids[view.Conversation.ID] = true
		for _, message := range view.Messages {
			if message.Kind == "initial_prompt" {
				prompts[message.Content]++
				if message.Content == "find archival files" {
					cmd := cliCommand(t, home, "history", "show", view.Conversation.ID, "--json")
					var diagnostics bytes.Buffer
					cmd.Stderr = &diagnostics
					output, err := cmd.Output()
					if err != nil {
						t.Fatalf("history show: %v; stderr: %s", err, diagnostics.String())
					}
					var shown historyView
					if err := json.Unmarshal(output, &shown); err != nil {
						t.Fatalf("history show is not JSON: %v; stdout: %s", err, output)
					}
					if shown.Conversation.ID != view.Conversation.ID || len(shown.Responses) != 1 ||
						len(shown.Responses[0].Commands) != 1 || shown.Responses[0].Commands[0].Command != "find . -type f" {
						t.Fatalf("history show lost migrated response: %s", output)
					}
				}
			}
		}
	}
	for _, prompt := range []string{"find archival files", "concurrent writer a", "concurrent writer b"} {
		if prompts[prompt] != 1 {
			t.Errorf("prompt %q occurred %d times, want once", prompt, prompts[prompt])
		}
	}
	assertMigrationFilesAndLedger(t, home)
}

func TestHistoryMigrationRecoversVerifiedLedger(t *testing.T) {
	t.Parallel()
	home := seedLegacyHistory(t)
	before := exportHistory(t, home)
	if len(before) != 1 {
		t.Fatalf("initial migration exported %d conversations, want 1", len(before))
	}
	assertMigrationFilesAndLedger(t, home)
	// Recreate the durable state after archive rename but before ledger activation.
	db := openHistoryDB(t, home)
	result, err := db.ExecContext(t.Context(), `UPDATE migration_runs SET status='verified',completed_at=NULL WHERE status='completed'`)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		t.Fatalf("prepare interrupted ledger: changed=%d error=%v", changed, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	after := exportHistory(t, home)
	if len(after) != 1 || after[0].Conversation.ID != before[0].Conversation.ID {
		t.Fatalf("recovery duplicated or replaced legacy conversation: %+v", after)
	}
	assertMigrationFilesAndLedger(t, home)
}

func seedLegacyHistory(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "history"), []byte(legacyLine), 0o644); err != nil { //nolint:gosec // Exercise automatic repair of historical wide file modes.
		t.Fatal(err)
	}
	return home
}

type historyView struct {
	Conversation struct{ ID string }
	Messages     []struct{ Kind, Content string }
	Responses    []struct {
		Commands []struct{ Command string }
	}
}

func exportHistory(t *testing.T, home string) []historyView {
	t.Helper()
	cmd := cliCommand(t, home, "history", "export", "--format=json")
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("history export: %v; stderr: %s", err, diagnostics.String())
	}
	var views []historyView
	if err := json.Unmarshal(output, &views); err != nil {
		t.Fatalf("history export is not JSON: %v; stdout: %s", err, output)
	}
	return views
}

func openHistoryDB(t *testing.T, home string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(home, ".zev", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func assertMigrationFilesAndLedger(t *testing.T, home string) {
	t.Helper()
	source := filepath.Join(home, ".zev", "history")
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy source remains active: %v", err)
	}
	archives, err := filepath.Glob(source + ".migrated.*.jsonl")
	if err != nil || len(archives) != 0 {
		t.Fatalf("legacy archives = %v, error = %v; migration must delete the source", archives, err)
	}
	db := openHistoryDB(t, home)
	for _, query := range []string{
		`SELECT count(*) FROM migration_runs`,
		`SELECT count(*) FROM migration_runs WHERE status='completed' AND completed_at IS NOT NULL`,
		`SELECT count(*) FROM migration_lines`,
	} {
		var count int
		if err := db.QueryRowContext(t.Context(), query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("%s = %d, want 1", query, count)
		}
	}
}
