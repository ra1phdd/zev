//go:build !windows

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const editorOperation = "editor"

func TestCancellationStopsDescendants(t *testing.T) {
	for _, operation := range []string{editorOperation, executeOperation} {
		t.Run(operation, func(t *testing.T) {
			setTestHome(t)
			svc, _ := newTestService(t)
			ready := filepath.Join(t.TempDir(), "ready")
			source := fmt.Sprintf("#!/bin/bash\nsleep 60 &\nchild=$!\nprintf '%%s' \"$child\" > %q\nmv %q %q\nwait \"$child\"\n", ready+".tmp", ready+".tmp", ready)
			content := []byte(source)
			if operation == editorOperation {
				content = []byte("#!/bin/bash\nprintf original\n")
				editor := filepath.Join(t.TempDir(), editorOperation)
				if err := os.WriteFile(editor, []byte(source), 0o700); err != nil { //nolint:gosec // The private editor fixture must be executable by its owner.
					t.Fatal(err)
				}
				t.Setenv("SCRIPT_EDITOR", editor)
			}
			if err := svc.repo.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile}, content); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				if operation == editorOperation {
					result <- svc.OpenEditor(ctx, safeScriptName)
				} else {
					result <- svc.Execute(ctx, safeScriptName, nil)
				}
			}()
			pid := waitForDescendant(t, ready, result)
			t.Cleanup(func() {
				if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
					t.Errorf("clean up fixture descendant: %v", err)
				}
			})
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("%s error = %v, want cancellation", operation, err)
				}
				if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr == nil {
					t.Errorf("%s error = %v, want child exit error", operation, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("operation did not join cancellation")
			}
			waitForDescendantExit(t, pid)
		})
	}
}

func waitForDescendant(t *testing.T, ready string, result <-chan error) int {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(filepath.Clean(ready))
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil || pid <= 0 {
				t.Fatalf("invalid descendant PID %q: %v", data, err)
			}
			if err := syscall.Kill(pid, 0); err != nil {
				t.Fatalf("fixture descendant not running: %v", err)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case err := <-result:
			t.Fatalf("operation exited before descendant was ready: %v", err)
		case <-deadline.C:
			t.Fatal("descendant did not become ready")
		case <-ticker.C:
		}
	}
}

func waitForDescendantExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatalf("check descendant exit: %v", err)
		}
		select {
		case <-deadline.C:
			t.Fatalf("descendant %d is still alive after cancellation", pid)
		case <-ticker.C:
		}
	}
}
