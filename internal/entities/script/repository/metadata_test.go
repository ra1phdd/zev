package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const newScriptName = "new"

func TestMetadataDirectorySyncFailureIsPostCommit(t *testing.T) {
	t.Parallel()
	r := New(testDir(t))
	if err := r.Create(t.Context(), domain.Script{Name: safeName, ScriptFile: safeFile}, []byte("original")); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected metadata directory sync failure")
	r.syncMetadataDirectory = func(*os.Root) error { return failure }
	if err := r.Create(t.Context(), domain.Script{Name: newScriptName, ScriptFile: "new.sh"}, []byte(newScriptName)); !errors.Is(err, ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("Create error = %v, want committed sync failure", err)
	}
	journalPath := filepath.Join(filepath.Dir(r.scriptsYAMLPath), ".scripts-journal", "journal")
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("journal not retained: %v", err)
	}
	r.syncMetadataDirectory = nil
	scripts, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scripts[newScriptName]; !ok {
		t.Fatalf("metadata replacement was rolled back: %#v", scripts)
	}
	data, err := os.ReadFile(filepath.Clean(filepath.Join(r.scriptsDir, "new.sh")))
	if err != nil || string(data) != newScriptName {
		t.Fatalf("created script was rolled back: %q, error = %v", data, err)
	}
}

func TestCreatePersistsCanonicalV2AndBodyEditRetainsRecordID(t *testing.T) {
	t.Parallel()
	r := New(testDir(t))
	if err := r.Create(t.Context(), domain.Script{Name: safeName, ScriptFile: safeFile, Description: "description", Arguments: []domain.Argument{{Name: "value", Required: true}}}, []byte("original")); err != nil {
		t.Fatal(err)
	}
	scripts, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	created := scripts[safeName]
	if created.RecordID == "" {
		t.Fatal("Create did not generate a record ID")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(r.scriptsYAMLPath), "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 2") || !strings.Contains(string(data), "recordID:") {
		t.Fatalf("metadata is not canonical v2: %q", data)
	}
	if err := r.ReplaceScript(t.Context(), ReplaceParams{Source: captureEditableSource(t, r), Body: []byte("edited")}); err != nil {
		t.Fatal(err)
	}
	scripts, err = r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got := scripts[safeName]; got.RecordID != created.RecordID || got.Arguments[0].Name != "value" {
		t.Fatalf("record metadata changed after body edit: %#v", got)
	}
}

func TestLegacyMetadataMigratesBeforeMutation(t *testing.T) {
	t.Parallel()
	r := New(testDir(t))
	dir := filepath.Dir(r.scriptsYAMLPath)
	if err := os.MkdirAll(filepath.Join(dir, "script_files"), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte("safe:\n  script: safe.sh\n  description: legacy\n")
	if err := os.WriteFile(filepath.Join(dir, "scripts"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script_files", "safe.sh"), []byte(scriptBody), 0o700); err != nil { //nolint:gosec // The fixture models executable script permissions.
		t.Fatal(err)
	}
	scripts, err := r.LoadAll()
	if err != nil || scripts[safeName].Description != "legacy" || scripts[safeName].RecordID == "" {
		t.Fatalf("legacy metadata read = %#v, %v", scripts, err)
	}
	if err := r.Create(t.Context(), domain.Script{Name: newScriptName, ScriptFile: "new.sh"}, []byte("new body")); err != nil {
		t.Fatal(err)
	}
	if err := r.Remove(safeName); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "scripts")) //nolint:gosec // The test reads its isolated TempDir fixture.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 2") {
		t.Fatalf("metadata was not canonicalized: %q", data)
	}
}

func TestRecoveryRejectsThirdPartyBody(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{scriptJournalKindCreate, scriptJournalKindReplace} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := testDir(t)
			filesDir := filepath.Join(dir, "script_files")
			if err := os.Mkdir(filesDir, 0o700); err != nil {
				t.Fatal(err)
			}
			bodyPath := filepath.Join(filesDir, safeFile)
			stageDir := filepath.Join(filesDir, ".replace-test")
			if err := os.Mkdir(stageDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bodyPath, []byte("third-party"), 0o700); err != nil { //nolint:gosec // The fixture models executable script permissions.
				t.Fatal(err)
			}
			stagePath := filepath.Join(stageDir, "script")
			if err := os.WriteFile(stagePath, []byte("replacement"), 0o700); err != nil { //nolint:gosec // The fixture models executable script permissions.
				t.Fatal(err)
			}
			files, err := os.OpenRoot(filesDir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := files.Close(); err != nil {
					t.Error(err)
				}
			}()
			st := &store{files: files}
			journal := scriptJournal{Kind: kind, OldBodyHash: hashBytes([]byte("original")), NewBodyHash: hashBytes([]byte("replacement"))}
			if err := st.recoverPreparedBody(journal, journalPaths{bodyPath: bodyPath, stagePath: stagePath}, kind == scriptJournalKindReplace); err == nil {
				t.Fatal("recovery accepted a third-party body")
			}
			data, err := os.ReadFile(bodyPath) //nolint:gosec // The test reads its isolated TempDir fixture.
			if err != nil || string(data) != "third-party" {
				t.Fatalf("third-party body changed: %q, %v", data, err)
			}
		})
	}
}
