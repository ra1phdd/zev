package repository

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func closeImportTest(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Error(err)
	}
}

func importTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, path
}

type legacyTestEntry struct {
	Timestamp       time.Time               `json:"timestamp"`
	Prompt          string                  `json:"prompt"`
	Response        *domain.OptionsResponse `json:"response"`
	Provider        string                  `json:"provider"`
	Model           string                  `json:"model"`
	SelectedCommand string                  `json:"selected_command,omitempty"` //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
	SelectedIndex   int                     `json:"selected_index,omitempty"`   //nolint:tagliatelle // Persisted legacy JSON contract requires snake_case field names.
}

func legacyTestLine(t *testing.T) []byte {
	t.Helper()
	entry := legacyTestEntry{
		Timestamp: time.Date(2024, 3, 2, 1, 2, 3, 456, time.FixedZone("offset", 3600)),
		Prompt:    "find archival files", Provider: "legacy-provider", Model: "legacy-model",
		Response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{
			{Command: "find . -type f", ShortExplanation: "archive explanation"},
			{Command: "rm archive", ShortExplanation: "remove archive", IsDangerous: true, DangerousExplanation: "irreversible removal"},
		}}, SelectedCommand: "rm archive", SelectedIndex: 1,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

func importTestCount(t *testing.T, s *Store, query string, want int) {
	t.Helper()
	var got int
	if err := s.db.QueryRowContext(t.Context(), query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", query, got, want)
	}
}

func TestImportLegacyCorruptionRollsBackWithoutPayload(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	source := filepath.Join(filepath.Dir(path), "history")
	original := append(legacyTestLine(t), []byte("{\"prompt\":\"TOP-SECRET\",\"timestamp\":\"TOP-SECRET\"}\n")...)
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	err := s.importLegacy(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "processed=1") || strings.Contains(err.Error(), "TOP-SECRET") {
		t.Fatalf("unsafe or missing corruption report: %v", err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 0)
	importTestCount(t, s, `SELECT count(*) FROM migration_runs`, 0)
	actual, err := os.ReadFile(source) //nolint:gosec // The test path is created inside t.TempDir().
	if err != nil || string(actual) != string(original) {
		t.Fatalf("source changed: %v", err)
	}
}

func TestImportLegacyRecoversAfterRenameBeforeActivation(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.WriteFile(source, legacyTestLine(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.importLegacy(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), `UPDATE migration_runs SET status='verified',completed_at=NULL`); err != nil {
		t.Fatal(err)
	}
	if err := s.importLegacy(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	importTestCount(t, s, `SELECT count(*) FROM migration_runs WHERE status='completed'`, 1)
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 1)
}

func TestImportLegacyConcurrentCallersDoNotDuplicate(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := os.WriteFile(filepath.Join(dir, "history"), legacyTestLine(t), 0o600); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- s.importLegacy(context.Background(), path) }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 1)
	importTestCount(t, s, `SELECT count(*) FROM migration_runs WHERE status='completed'`, 1)
}

func TestImportLegacyPreservesRecordsAndArchives(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	line := legacyTestLine(t)
	source := filepath.Join(filepath.Dir(path), "history")
	original := append(append([]byte{}, line...), line...)
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.importLegacy(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		t.Fatalf("source still active: %v", err)
	}
	archives, err := filepath.Glob(source + ".migrated.*.jsonl")
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives = %v, %v", archives, err)
	}
	archived, err := os.ReadFile(archives[0])
	if err != nil || string(archived) != string(original) {
		t.Fatalf("archive changed: %v", err)
	}
	info, err := os.Stat(archives[0])
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("archive permissions: %v %v", info, err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 2)
	importTestCount(t, s, `SELECT count(DISTINCT id) FROM conversations`, 2)
	importTestCount(t, s, `SELECT count(*) FROM messages WHERE kind='initial_prompt'`, 2)
	importTestCount(t, s, `SELECT count(*) FROM messages WHERE kind='refinement'`, 0)
	importTestCount(t, s, `SELECT count(*) FROM responses`, 2)
	importTestCount(t, s, `SELECT count(*) FROM commands`, 4)
	importTestCount(t, s, `SELECT count(*) FROM message_embeddings`, 0)
	importTestCount(t, s, `SELECT count(*) FROM migration_runs WHERE status='completed' AND completed_at IS NOT NULL`, 1)
	importTestCount(t, s, `SELECT count(*) FROM migration_lines`, 2)
	importTestCount(t, s, `SELECT count(*) FROM history_fts WHERE history_fts MATCH 'archival'`, 2)
	rows, err := s.ListConversations(context.Background(), service.ListRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rows {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(c.MetadataJSON), &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata["source"] != "legacy_jsonl" || metadata["source_line"] == nil || metadata["line_fingerprint"] == nil {
			t.Fatalf("metadata = %v", metadata)
		}
		view, err := s.GetConversation(context.Background(), c.ID)
		if err != nil {
			t.Fatal(err)
		}
		const completedStatus = "completed"
		if c.Status != completedStatus || c.Provider != "legacy-provider" || len(view.Messages) != 2 || len(view.Responses) != 1 {
			t.Fatalf("view = %+v", view)
		}
		if view.Messages[0].Content != "find archival files" || view.Messages[0].Sequence != 0 || view.Messages[1].Sequence != 1 {
			t.Fatalf("messages = %+v", view.Messages)
		}
		response := view.Responses[0]
		if len(response.Commands) != 2 || c.SelectedCommandID != response.Commands[1].ID || response.Commands[1].SelectedAt == nil || response.Commands[1].ExecutedAt != nil || response.Commands[1].ExecutionExitCode != nil {
			t.Fatalf("selection = %+v", response.Commands)
		}
		var payload domain.OptionsResponse
		if err := json.Unmarshal([]byte(response.PayloadJSON), &payload); err != nil || payload.Commands[1].DangerousExplanation != "irreversible removal" {
			t.Fatalf("payload = %+v %v", payload, err)
		}
		wantTime := time.Date(2024, 3, 2, 0, 2, 3, 456, time.UTC)
		if !c.CreatedAt.Equal(wantTime) {
			t.Fatalf("timestamp = %v", c.CreatedAt)
		}
	}
	if err := s.importLegacy(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 2)
}
