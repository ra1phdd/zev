package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const importTestUserRole = "user"

func prepareVerifiedImport(t *testing.T, s *Store, path string) legacyRun {
	t.Helper()
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.WriteFile(source, legacyTestLine(t), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openLegacySource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, file) })
	snapshot, err := fingerprintLegacy(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	run := legacyRun{ID: uuid.NewString(), Fingerprint: snapshot.fingerprint(), Metadata: legacyRunMetadata{
		FormatVersion: 1, legacySnapshot: snapshot,
	}}
	if err := s.importLegacySnapshot(t.Context(), file, source, &run); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestImportLegacyResumeRejectsInvalidLedgerBeforeSourceRemoval(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{
		`DELETE FROM migration_lines`,
		`UPDATE migration_runs SET metadata_json=json_set(metadata_json,'$.line_count',99)`,
	} {
		t.Run(mutation, func(t *testing.T) {
			s, path := importTestStore(t)
			prepareVerifiedImport(t, s, path)
			if _, err := s.db.ExecContext(t.Context(), mutation); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(t.Context(), path, false); !errors.Is(err, service.ErrConflict) {
				t.Fatalf("resume = %v, want conflict", err)
			}
			source := filepath.Join(filepath.Dir(path), "history")
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source was removed before ledger verification: %v", err)
			}
			importTestCount(t, s, `SELECT count(*) FROM migration_runs WHERE status='completed'`, 0)
		})
	}
}

func TestImportLegacyExplicitCall(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "history")
	original := bytes.Repeat(legacyTestLine(t), 2)
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	opened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	importTestCount(t, opened, `SELECT count(*) FROM conversations`, 0)
	if _, err := os.Stat(source); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err = Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, opened) })
	importTestCount(t, opened, `SELECT count(DISTINCT id) FROM conversations`, 2)
	importTestCount(t, opened, `SELECT count(*) FROM migration_runs WHERE status='completed'`, 1)
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists after migration: %v", err)
	}
}

func TestImportLegacyChangedSourcePreservesUserRecords(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	_, err := s.StartConversation(t.Context(), service.StartConversationInput{
		Conversation:   service.Conversation{ID: "user-owned", CreatedAt: time.Now()},
		InitialMessage: service.Message{ID: "user-message", Role: importTestUserRole, Kind: initialPromptKind, Content: "keep this"},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepareVerifiedImport(t, s, path)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.WriteFile(source, bytes.Repeat(legacyTestLine(t), 2), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, opened) })
	importTestCount(t, opened, `SELECT count(*) FROM conversations`, 3)
	importTestCount(t, opened, `SELECT count(*) FROM conversations WHERE id='user-owned'`, 1)
	importTestCount(t, opened, `SELECT count(*) FROM migration_lines`, 2)
	importTestCount(t, opened, `SELECT count(*) FROM migration_runs`, 1)
}

func TestImportLegacySourceChangeRollsBackSnapshot(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.WriteFile(source, legacyTestLine(t), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openLegacySource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, file) })
	snapshot, err := fingerprintLegacy(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, bytes.Repeat(legacyTestLine(t), 2), 0o600); err != nil {
		t.Fatal(err)
	}
	run := legacyRun{ID: uuid.NewString(), Fingerprint: snapshot.fingerprint(), Metadata: legacyRunMetadata{FormatVersion: 1, legacySnapshot: snapshot}}
	if err := s.importLegacySnapshot(t.Context(), file, source, &run); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("changed snapshot = %v", err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 0)
	importTestCount(t, s, `SELECT count(*) FROM migration_runs`, 0)
}

func TestImportLegacyCompletedSourceConflict(t *testing.T) {
	t.Parallel()
	s, path := importTestStore(t)
	run := prepareVerifiedImport(t, s, path)
	if err := Migrate(t.Context(), path, false); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(filepath.Dir(path), "history")
	if err := os.WriteFile(source, []byte("different source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), path, false); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("completed source conflict = %v", err)
	}
	var metadata legacyRunMetadata
	encoded, err := json.Marshal(run.Metadata)
	if err != nil || json.Unmarshal(encoded, &metadata) != nil {
		t.Fatal(err)
	}
	importTestCount(t, s, `SELECT count(*) FROM conversations`, 1)
}
