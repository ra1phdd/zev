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

func createEditableScript(t *testing.T) (*Repository, *os.File) {
	t.Helper()
	r := New(testDir(t))
	if err := r.Create(t.Context(), domain.Script{Name: safeName, ScriptFile: safeFile}, []byte("original complete script")); err != nil {
		t.Fatal(err)
	}
	original, err := r.OpenScript(t.Context(), safeFile)
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

func TestReplaceScriptRejectsConcurrentStaleEditors(t *testing.T) {
	t.Parallel()
	r, original := createEditableScript(t)
	second := New(filepath.Dir(r.scriptsYAMLPath))
	stale, err := second.OpenScript(t.Context(), safeFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stale.Close(); err != nil {
			t.Error(err)
		}
	})
	type result struct {
		content string
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, file := range []*os.File{original, stale} {
		wg.Go(func() {
			<-start
			content := []string{"first edit", "second edit"}[i]
			repo := []*Repository{r, second}[i]
			results <- result{content: content, err: repo.ReplaceScript(t.Context(), safeFile, file, []byte(content))}
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
	if err != nil || string(data) != "original complete script" {
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
			r, original := createEditableScript(t)
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
			if err := r.ReplaceScript(t.Context(), safeFile, original, []byte("replacement")); err == nil {
				t.Fatal("replacement succeeded despite injected failure")
			}
			data, err := os.ReadFile(filepath.Clean(filepath.Join(r.scriptsDir, safeFile)))
			if err != nil || string(data) != "original complete script" {
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
