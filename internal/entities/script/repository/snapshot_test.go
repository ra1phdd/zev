package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
)

const (
	snapshotRecordIDDrift     = "record-id"
	snapshotRestoredStatDrift = "restored-stat"
)

func TestReadBodyRejectsMetadataDriftFromGenerationSnapshot(t *testing.T) {
	t.Parallel()
	r, _ := createEditableScript(t)
	changed, source, err := r.CaptureScript(t.Context(), safeName)
	if err != nil {
		t.Fatal(err)
	}
	changed.Description = "changed after capture"
	if err := r.Update(t.Context(), UpdateParams{Change: func(scripts map[string]domain.Script) error {
		scripts[safeName] = changed
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadBody(t.Context(), ReadBodyParams{Snapshot: source}); !errors.Is(err, ErrBodyChanged) {
		t.Fatalf("generation snapshot accepted metadata drift: %v", err)
	}
}

func TestReplacementRejectsZeroSnapshot(t *testing.T) {
	t.Parallel()
	r, _ := createEditableScript(t)
	if err := r.ReplaceScript(t.Context(), ReplaceParams{Body: []byte("unbound edit")}); !errors.Is(err, ErrBodyChanged) {
		t.Fatalf("invalid prerequisite not classified as stale: %v", err)
	}
}

func TestReplacementRechecksSourceAfterStaging(t *testing.T) {
	t.Parallel()
	for _, drift := range []string{"in-place", snapshotRestoredStatDrift, "same-content-inode", snapshotRecordIDDrift, "record-path"} {
		t.Run(drift, func(t *testing.T) {
			t.Parallel()
			r, _ := createEditableScript(t)
			source := captureEditableSource(t, r)
			path := filepath.Join(r.scriptsDir, safeFile)
			wantBody := editableBodyFixture
			r.replaceWrite = func(file *os.File, data []byte) (int, error) {
				switch drift {
				case "in-place", snapshotRestoredStatDrift:
					wantBody = "concurrent body"
					if drift == snapshotRestoredStatDrift {
						wantBody = strings.Repeat("x", len(editableBodyFixture))
					}
					if err := os.WriteFile(path, []byte(wantBody), 0o600); err != nil {
						t.Fatal(err)
					}
					if drift == snapshotRestoredStatDrift {
						if err := os.Chtimes(path, source.info.ModTime(), source.info.ModTime()); err != nil {
							t.Fatal(err)
						}
						info, err := os.Stat(path)
						if err != nil || !sameBodyInfo(info, source.info) {
							t.Fatalf("fixture failed to restore stat: %v", err)
						}
					}
				case "same-content-inode":
					replacement := path + ".new"
					if err := os.WriteFile(replacement, []byte(wantBody), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(replacement, source.info.Mode().Perm()); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(replacement, source.info.ModTime(), source.info.ModTime()); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, path); err != nil {
						t.Fatal(err)
					}
				case snapshotRecordIDDrift, "record-path":
					all, err := os.ReadFile(r.scriptsYAMLPath)
					if err != nil {
						t.Fatal(err)
					}
					scripts, err := metadata.DecodeScripts(all)
					if err != nil {
						t.Fatal(err)
					}
					script := scripts[safeName]
					if drift == snapshotRecordIDDrift {
						script.RecordID = uuid.NewString()
					} else {
						script.ScriptFile = "moved.sh"
					}
					scripts[safeName] = script
					encoded, err := metadata.EncodeScriptsV2(scripts)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Clean(r.scriptsYAMLPath), encoded, 0o600); err != nil { //nolint:gosec // Decoded fixture data cannot change the repository-owned metadata path.
						t.Fatal(err)
					}
				}
				n, err := file.Write(data)
				if err != nil {
					t.Fatal(err)
				}
				return n, nil
			}
			err := r.ReplaceScript(t.Context(), ReplaceParams{Source: source, Body: []byte("stale edit")})
			if !errors.Is(err, ErrBodyChanged) {
				t.Fatalf("stale source accepted after %s: %v", drift, err)
			}
			if body, err := os.ReadFile(filepath.Clean(path)); err != nil || string(body) != wantBody {
				t.Fatalf("concurrent body lost: %q, %v", body, err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(r.scriptsYAMLPath), ".scripts-journal", "journal")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejection left a journal: %v", err)
			}
		})
	}
}

func TestUpdateRechecksBodyBeforePublication(t *testing.T) {
	t.Parallel()
	r, _ := createEditableScript(t)
	source := captureEditableSource(t, r)
	before, err := os.ReadFile(r.scriptsYAMLPath)
	if err != nil {
		t.Fatal(err)
	}
	err = r.Update(t.Context(), UpdateParams{Source: &source, Change: func(all map[string]domain.Script) error {
		script := all[safeName]
		script.Description = "stale metadata"
		all[safeName] = script
		return os.WriteFile(filepath.Join(r.scriptsDir, safeFile), []byte("concurrent body"), 0o600)
	}})
	if !errors.Is(err, ErrBodyChanged) {
		t.Fatalf("metadata published after body drift: %v", err)
	}
	after, err := os.ReadFile(r.scriptsYAMLPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("rejected metadata changed: %q, %v", after, err)
	}
}
