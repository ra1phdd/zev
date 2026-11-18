package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestEditorUsesIsolatedCopy(t *testing.T) {
	svc, dir := newTestService(t)
	if err := svc.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile}, []byte("original\n")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(t.TempDir(), "editor")
	scriptPath := filepath.Join(dir, "script_files", safeScriptFile)
	source := fmt.Sprintf("#!/bin/bash\nset -eu\nmv %q %q\nln -s %q %q\nprintf 'edited\\n' > \"$1\"\n", scriptPath, scriptPath+".old", outside, scriptPath)
	if err := os.WriteFile(editor, []byte(source), 0o700); err != nil { //nolint:gosec // The private editor fixture must be executable by its owner.
		t.Fatal(err)
	}
	t.Setenv("SCRIPT_EDITOR", editor)
	if err := svc.OpenEditor(t.Context(), safeScriptName); err == nil {
		t.Fatal("accepted replaced destination")
	}
	if data, err := os.ReadFile(filepath.Clean(outside)); err != nil || string(data) != "untouched" {
		t.Fatalf("external sentinel changed: %q %v", data, err)
	}
}

func TestEditorRejectsInPlaceSourceRewrite(t *testing.T) {
	svc, dir := newTestService(t)
	if err := svc.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile}, []byte("original\n")); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "script_files", safeScriptFile)
	editor := filepath.Join(t.TempDir(), "editor")
	source := fmt.Sprintf("#!/bin/sh\nset -eu\nprintf 'concurrent\\n' > %q\nprintf 'stale edit\\n' > \"$1\"\n", scriptPath)
	if err := os.WriteFile(editor, []byte(source), 0o700); err != nil { //nolint:gosec // The private editor fixture must be executable.
		t.Fatal(err)
	}
	t.Setenv("SCRIPT_EDITOR", editor)
	if err := svc.OpenEditor(t.Context(), safeScriptName); err == nil {
		t.Fatal("stale editor overwrote an in-place body rewrite")
	}
	if body, err := os.ReadFile(filepath.Clean(scriptPath)); err != nil || string(body) != "concurrent\n" {
		t.Fatalf("concurrent body lost: %q, %v", body, err)
	}
}

func TestEditorCommitsAtomicSave(t *testing.T) {
	svc, dir := newTestService(t)
	if err := svc.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile}, []byte("original\n")); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(t.TempDir(), "editor")
	source := "#!/bin/bash\nset -eu\nprintf '#!/bin/bash\\nprintf edited\\n' > \"$1.new\"\nmv \"$1.new\" \"$1\"\n"
	if err := os.WriteFile(editor, []byte(source), 0o700); err != nil { //nolint:gosec // The private editor fixture must be executable by its owner.
		t.Fatal(err)
	}
	t.Setenv("SCRIPT_EDITOR", editor)
	if err := svc.OpenEditor(t.Context(), safeScriptName); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "script_files", safeScriptFile)))
	if err != nil || string(data) != "#!/bin/bash\nprintf edited\n" {
		t.Fatalf("edit not saved: %q %v", data, err)
	}
}

func TestEditorCancelledContext(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	if err := svc.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile}, []byte("original\n")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := svc.OpenEditor(ctx, safeScriptName); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenEditor: %v", err)
	}
}

//nolint:paralleltest // This test temporarily replaces the process-global os.Stdin.
func TestExecutePreservesArgumentsAndStdin(t *testing.T) {
	svc, _ := newTestService(t)
	out := filepath.Join(t.TempDir(), "result")
	source := []byte("#!/bin/bash\nread -r line\nprintf '%s|%s|%s' \"$1\" \"$2\" \"$line\" > \"$3\"\n")
	if err := svc.repo.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile}, source); err != nil {
		t.Fatal(err)
	}
	input, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := input.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := input.WriteString("stdin value\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = old })
	if err := svc.Execute(t.Context(), safeScriptName, []string{"arg space", ";literal", out}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Clean(out)); err != nil || string(data) != "arg space|;literal|stdin value" {
		t.Fatalf("execution: %q %v", data, err)
	}
}
