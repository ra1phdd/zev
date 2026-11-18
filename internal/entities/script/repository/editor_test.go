package repository

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const editableBodyFixture = "original complete script"

func createEditableScript(t *testing.T) (*Repository, *os.File) {
	t.Helper()
	r := New(testDir(t))
	if err := r.Create(t.Context(), domain.Script{Name: safeName, ScriptFile: safeFile}, []byte(editableBodyFixture)); err != nil {
		t.Fatal(err)
	}
	script, _, err := r.CaptureScript(t.Context(), safeName)
	if err != nil {
		t.Fatal(err)
	}
	original, err := r.OpenScriptPinned(t.Context(), script, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := original.Close(); err != nil {
			t.Error(err)
		}
	})
	return r, original
}

func captureEditableSource(t *testing.T, r *Repository) BodySnapshot {
	t.Helper()
	_, source, err := r.CaptureScript(t.Context(), safeName)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestOpenScriptPinnedHoldsStoreLockThroughOpen(t *testing.T) {
	t.Parallel()
	r, _ := createEditableScript(t)
	all, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	script := all[safeName]
	removeStarted := make(chan struct{})
	removeDone := make(chan error, 1)
	file, err := r.OpenScriptPinned(t.Context(), script, func(domain.Script) error {
		go func() {
			close(removeStarted)
			removeDone <- r.Remove(safeName)
		}()
		<-removeStarted
		select {
		case err := <-removeDone:
			return fmt.Errorf("remove completed before pinned open returned: %w", err)
		default:
			return nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(file)
	if closeErr := file.Close(); err != nil || closeErr != nil || string(body) != editableBodyFixture {
		t.Fatalf("pinned body = %q, read=%v close=%v", body, err, closeErr)
	}
	if err := <-removeDone; err != nil {
		t.Fatal(err)
	}
}

func TestReplaceScriptRejectsConcurrentStaleEditors(t *testing.T) {
	t.Parallel()
	r, original := createEditableScript(t)
	source := captureEditableSource(t, r)
	second := New(filepath.Dir(r.scriptsYAMLPath))
	type result struct {
		content string
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			<-start
			content := []string{"first edit", "second edit"}[i]
			repo := []*Repository{r, second}[i]
			results <- result{content: content, err: repo.ReplaceScript(t.Context(), ReplaceParams{Source: source, Body: []byte(content)})}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	var successes int
	var winner string
	for result := range results {
		if result.err == nil {
			successes++
			winner = result.content
		}
	}
	if successes != 1 {
		t.Fatalf("successful stale editors = %d, want exactly one", successes)
	}
	data, err := os.ReadFile(filepath.Clean(filepath.Join(r.scriptsDir, safeFile)))
	if err != nil || string(data) != winner {
		t.Fatalf("winning edit = %q, error = %v, want %q", data, err, winner)
	}
	data, err = io.ReadAll(original)
	if err != nil || string(data) != editableBodyFixture {
		t.Fatalf("pinned original changed: %q, error = %v", data, err)
	}
	info, err := os.Stat(filepath.Join(r.scriptsDir, safeFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Errorf("edited script permissions = %v, want 0700", info.Mode())
	}
}

func TestReplaceScriptFailurePreservesPreviousFile(t *testing.T) {
	t.Parallel()
	failure := errors.New("injected failure")
	for _, stage := range []string{"write", "sync", "rename"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			r, _ := createEditableScript(t)
			source := captureEditableSource(t, r)
			r.replaceWrite = (*os.File).Write
			r.replaceSync = (*os.File).Sync
			r.replaceRename = (*os.Root).Rename
			switch stage {
			case "write":
				r.replaceWrite = func(file *os.File, data []byte) (int, error) {
					if _, err := file.Write(data[:1]); err != nil {
						return 1, fmt.Errorf("injected write: %w", err)
					}
					return 1, failure
				}
			case "sync":
				r.replaceSync = func(*os.File) error { return failure }
			case "rename":
				r.replaceRename = func(*os.Root, string, string) error { return failure }
			}
			if err := r.ReplaceScript(t.Context(), ReplaceParams{Source: source, Body: []byte("replacement")}); err == nil {
				t.Fatal("replacement succeeded despite injected failure")
			}
			data, err := os.ReadFile(filepath.Clean(filepath.Join(r.scriptsDir, safeFile)))
			if err != nil || string(data) != editableBodyFixture {
				t.Fatalf("previous file changed: %q, error = %v", data, err)
			}
			entries, err := os.ReadDir(r.scriptsDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Errorf("staged file leaked: %v", entries)
			}
		})
	}
}
