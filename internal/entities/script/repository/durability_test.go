package repository

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestRemovalRetainsRecoveryArtifactsUntilMetadataSyncSucceeds(t *testing.T) {
	t.Parallel()
	r, _ := createEditableScript(t)
	failure := errors.New("metadata directory unavailable")
	r.syncMetadataDirectory = func(*os.Root) error { return failure }
	if err := r.Remove(safeName); !errors.Is(err, ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("Remove = %v", err)
	}
	journalPath := filepath.Join(filepath.Dir(r.scriptsYAMLPath), ".scripts-journal", "journal")
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("recovery journal lost after failed sync: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(r.scriptsDir, ".scripts-remove-*", "body"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("removal backup lost: %v, %v", backups, err)
	}
	if _, err := r.LoadAll(); !errors.Is(err, ErrCommitted) {
		t.Fatalf("unresolved sync did not block access: %v", err)
	}
	if _, err := os.Stat(backups[0]); err != nil {
		t.Fatalf("retry discarded backup before sync: %v", err)
	}
	r.syncMetadataDirectory = nil
	if all, err := r.LoadAll(); err != nil || len(all) != 0 {
		t.Fatalf("recovery = %v, %v", all, err)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed recovery retained journal: %v", err)
	}
}

func TestPostBodyPublicationFailureRetainsJournal(t *testing.T) {
	t.Parallel()
	r, _ := createEditableScript(t)
	failure := errors.New("injected post-publication failure")
	r.syncMetadataDirectory = func(*os.Root) error { return failure }
	source := captureEditableSource(t, r)
	if err := r.ReplaceScript(t.Context(), ReplaceParams{Source: source, Body: []byte("replacement")}); !errors.Is(err, ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("post-publication failure = %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(r.scriptsYAMLPath), ".scripts-journal", "journal")); err != nil {
		t.Fatalf("post-publication failure lost journal: %v", err)
	}
}

func TestPreparedBodyAdoptionSyncFailureRetainsState(t *testing.T) {
	t.Parallel()
	dir := testDir(t)
	filesDir := filepath.Join(dir, "script_files")
	stageDir := filepath.Join(filesDir, ".replace-test")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bodyPath := filepath.Join(filesDir, safeFile)
	if err := os.WriteFile(bodyPath, []byte("replacement"), 0o700); err != nil { //nolint:gosec // The fixture models executable script permissions.
		t.Fatal(err)
	}
	files, err := os.OpenRoot(filesDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := files.Close(); err != nil {
			t.Error(err)
		}
	})
	failure := errors.New("body directory sync failed")
	var synced []string
	st := &store{files: files, syncBodyDirectory: func(_ *os.Root, relative string) error {
		synced = append(synced, relative)
		if relative == "." {
			return failure
		}
		return nil
	}}
	journal := scriptJournal{Kind: scriptJournalKindReplace, OldBodyHash: hashBytes([]byte(editableBodyFixture)), NewBodyHash: hashBytes([]byte("replacement"))}
	err = st.recoverPreparedBody(journal, journalPaths{
		bodyPath:  bodyPath,
		stagePath: filepath.Join(stageDir, "script"),
	}, true)
	if !errors.Is(err, ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("adoption error = %v", err)
	}
	if len(synced) != 2 || synced[0] != ".replace-test" || synced[1] != "." {
		t.Fatalf("synced directories = %v", synced)
	}
	data, readErr := os.ReadFile(bodyPath) //nolint:gosec // The fixture is isolated under t.TempDir.
	if readErr != nil || string(data) != "replacement" {
		t.Fatalf("adopted body changed after failed sync: %q, %v", data, readErr)
	}
}

func TestPreparedRemovalAdoptionSyncFailureRetainsState(t *testing.T) {
	t.Parallel()
	dir := testDir(t)
	filesDir := filepath.Join(dir, "script_files")
	backupDir := filepath.Join(filesDir, ".scripts-remove-test")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(backupDir, "body")
	if err := os.WriteFile(backupPath, []byte(editableBodyFixture), 0o700); err != nil { //nolint:gosec // The fixture models executable script permissions.
		t.Fatal(err)
	}
	files, err := os.OpenRoot(filesDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := files.Close(); err != nil {
			t.Error(err)
		}
	})
	failure := errors.New("backup directory sync failed")
	var synced []string
	st := &store{files: files, syncBodyDirectory: func(_ *os.Root, relative string) error {
		synced = append(synced, relative)
		if relative == ".scripts-remove-test" {
			return failure
		}
		return nil
	}}
	journal := scriptJournal{Kind: scriptJournalKindRemove, OldBodyHash: hashBytes([]byte(editableBodyFixture))}
	err = st.recoverPreparedRemove(journal, journalPaths{
		bodyPath:   filepath.Join(filesDir, safeFile),
		backupPath: backupPath,
	})
	if !errors.Is(err, ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("removal adoption error = %v", err)
	}
	if len(synced) != 2 || synced[0] != "." || synced[1] != ".scripts-remove-test" {
		t.Fatalf("synced directories = %v", synced)
	}
	if _, statErr := os.Stat(filepath.Join(filesDir, safeFile)); !os.IsNotExist(statErr) {
		t.Fatalf("removed body was recreated: %v", statErr)
	}
}

func TestJournalRejectsNonPrivateDirectoryAndLeaves(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode and symlink fixtures")
	}
	for _, kind := range []string{"directory-symlink", "directory-mode", "leaf-symlink", "leaf-mode", "leaf-hardlink", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := testDir(t)
			journalDir := filepath.Join(dir, ".scripts-journal")
			outside := t.TempDir()
			if kind == "directory-symlink" {
				if err := os.Symlink(outside, journalDir); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(journalDir, 0o700); err != nil {
					t.Fatal(err)
				}
				leaf := filepath.Join(journalDir, "journal")
				switch kind {
				case "directory-mode":
					if err := os.Chmod(journalDir, 0o755); err != nil { //nolint:gosec // Deliberately unsafe mode tests refusal.
						t.Fatal(err)
					}
				case "leaf-symlink":
					if err := os.WriteFile(filepath.Join(outside, "journal"), nil, 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(filepath.Join(outside, "journal"), leaf); err != nil {
						t.Fatal(err)
					}
				case "leaf-mode":
					if err := os.WriteFile(leaf, nil, 0o644); err != nil { //nolint:gosec // Deliberately unsafe mode tests refusal.
						t.Fatal(err)
					}
				case "leaf-hardlink":
					if err := os.WriteFile(filepath.Join(outside, "journal"), nil, 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Link(filepath.Join(outside, "journal"), leaf); err != nil {
						t.Fatal(err)
					}
				case "oversized":
					if err := os.WriteFile(leaf, []byte(strings.Repeat(" ", maxScriptJournalPayload+1)), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			r := New(dir)
			if err := r.Create(t.Context(), domain.Script{Name: safeName, ScriptFile: safeFile}, []byte("body")); err == nil {
				t.Fatalf("unsafe journal %s accepted", kind)
			}
			entries, err := os.ReadDir(outside)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".journal-") {
					t.Fatal("journal write escaped repository")
				}
			}
		})
	}
}
