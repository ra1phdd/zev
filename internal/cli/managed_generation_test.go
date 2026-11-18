package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	scriptservice "github.com/ra1phdd/zev/internal/entities/script/service"
	"github.com/spf13/cobra"
)

const metadataResponseMessage = "message"

//nolint:paralleltest // newCLIConfigHome sets process-wide HOME and USERPROFILE.
func TestManagedGenerationExplicitConsentAndFallback(t *testing.T) {
	home := newCLIConfigHome(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		for _, forbidden := range []string{home, "metadata-private-history", "metadata-private-context"} {
			if strings.Contains(string(data), forbidden) {
				t.Errorf("unrelated context transmitted: %s", forbidden)
			}
		}
		if call == 2 {
			http.Error(w, "provider unavailable", http.StatusBadRequest)
			return
		}
		content := `{"description":"Generated description"}`
		if strings.Contains(string(data), "field=arguments") {
			content = `{"arguments":[{"name":"target","required":false}]}`
		}
		response := map[string]any{"choices": []any{map[string]any{metadataResponseMessage: map[string]string{"content": content}}}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	writeCLIConfig(t, home, openAITestConfig(server.URL, "fixture"))

	if _, err := managedCommand(t, managedKindAlias, managedOpAdd, "manual", "echo", "--no-metadata"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("ordinary CRUD contacted generator")
	}
	out, err := managedCommand(t, managedKindAlias, managedOpAdd, "generated", managedTemplateFixture, "--generate-description", historyJSONFlag)
	if err != nil || !strings.Contains(out, "Generated description") {
		t.Fatalf("generation: %s %v", out, err)
	}
	out, err = managedCommand(t, managedKindAlias, managedOpEdit, "generated", "--generate-description", historyJSONFlag)
	if err != nil || !strings.Contains(out, "Generated description") {
		t.Fatalf("fallback did not preserve old metadata: %s %v", out, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
	out, err = managedCommand(t, managedKindAlias, managedOpEdit, "generated", "--generate-arguments", historyJSONFlag)
	if err != nil || !strings.Contains(out, `"required":false`) || !strings.Contains(out, "Generated description") {
		t.Fatalf("argument generation: %s %v", out, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestManagedScriptGenerationUsesFinalEditedBody(t *testing.T) {
	home := newCLIConfigHome(t)
	editor := filepath.Join(home, "editor.sh")
	const finalBody = "#!/usr/bin/env bash\necho final-editor-body-marker\n"
	editorSource := "#!/bin/sh\ncat > \"$1\" <<'EOF'\n" + finalBody + "EOF\n"
	if err := os.WriteFile(editor, []byte(editorSource), 0o700); err != nil { //nolint:gosec // The private editor fixture must be executable.
		t.Fatal(err)
	}

	// Fail promptly instead of opening a real editor if the configured editor is ignored.
	t.Setenv("EDITOR", "false")
	t.Setenv("SKIP_EDITOR", "")
	oldTerminal := managedTerminal
	managedTerminal = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { managedTerminal = oldTerminal })

	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		requestBody = string(data)
		if _, err := io.WriteString(w, `{"choices":[{"message":{"content":"{\"description\":\"generated from final body\"}"}}]}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	cfg := openAITestConfig(server.URL, "fixture")
	cfg.ScriptEditor = editor
	writeCLIConfig(t, home, cfg)

	if _, err := managedCommand(t, managedKindScript, managedOpCreate, "edited", "--generate-description"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(requestBody, "final-editor-body-marker") {
		t.Fatalf("generation request did not use final editor body: %s", requestBody)
	}
	body, err := os.ReadFile(filepath.Clean(filepath.Join(home, ".zev", "script_files", "edited.sh")))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != finalBody {
		t.Fatalf("saved body = %q, want %q", body, finalBody)
	}
	out, err := managedCommand(t, managedKindScript, managedOpList, historyJSONFlag)
	if err != nil || !strings.Contains(out, "generated from final body") {
		t.Fatalf("generated metadata was not persisted: %s %v", out, err)
	}
}

type managedSnapshotReader struct {
	reader     io.Reader
	beforeRead func()
}

func (r *managedSnapshotReader) Read(p []byte) (int, error) {
	if r.beforeRead != nil {
		r.beforeRead()
		r.beforeRead = nil
	}
	n, err := r.reader.Read(p)
	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}
	if err != nil {
		return n, fmt.Errorf("read snapshot fixture: %w", err)
	}
	return n, nil
}

func TestManagedScriptEditCapturesSourceBeforeInput(t *testing.T) {
	old := managedTerminal
	managedTerminal = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { managedTerminal = old })
	for _, mode := range []string{"metadata-form", "body-stdin"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			bodyFile := filepath.Join(home, "initial.sh")
			if err := os.WriteFile(bodyFile, []byte("echo initial\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := managedCommand(t, managedKindScript, managedOpCreate, "input-drift", "--body-file", bodyFile); err != nil {
				t.Fatal(err)
			}
			metadataPath := filepath.Join(home, ".zev", "scripts")
			before, err := os.ReadFile(filepath.Clean(metadataPath))
			if err != nil {
				t.Fatal(err)
			}
			bodyPath := filepath.Join(home, ".zev", "script_files", "input-drift.sh")
			root := NewRootCommand("test")
			var out, diagnostic bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&diagnostic)
			args := []string{managedKindScript, managedOpEdit, "input-drift"}
			input := "updated description\n[]\n"
			if mode == "body-stdin" {
				args = append(args, "--body-stdin")
				input = "echo stale replacement\n"
			}
			read := false
			root.SetIn(&managedSnapshotReader{reader: strings.NewReader(input), beforeRead: func() {
				read = true
				if err := os.WriteFile(bodyPath, []byte("echo concurrent\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}})
			root.SetArgs(args)
			if err := root.ExecuteContext(t.Context()); !errors.Is(err, scriptservice.ErrScriptChanged) || !read {
				t.Fatalf("stale %s not rejected after input: %v (read=%t)", mode, err, read)
			}
			after, err := os.ReadFile(filepath.Clean(metadataPath))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected edit changed metadata: %v", err)
			}
			body, err := os.ReadFile(filepath.Clean(bodyPath))
			if err != nil || string(body) != "echo concurrent\n" {
				t.Fatalf("concurrent body overwritten: %q, %v", body, err)
			}
		})
	}
}

//nolint:paralleltest // newCLIConfigHome sets process-wide HOME and USERPROFILE.
func TestManagedScriptGenerationRejectsBodyDriftDuringGeneration(t *testing.T) {
	home := newCLIConfigHome(t)
	bodyFile := filepath.Join(home, "initial.sh")
	if err := os.WriteFile(bodyFile, []byte("echo initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := managedCommand(t, managedKindScript, managedOpCreate, "drift", "--body-file", bodyFile); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".zev", "script_files", "drift.sh"), []byte("echo changed during generation\n"), 0o600); err != nil {
			t.Error(err)
		}
		if _, err := io.WriteString(w, `{"choices":[{"message":{"content":"{\"description\":\"generated\"}"}}]}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	writeCLIConfig(t, home, openAITestConfig(server.URL, "fixture"))

	if _, err := managedCommand(t, managedKindScript, managedOpEdit, "drift", "--generate-description"); !errors.Is(err, scriptservice.ErrScriptChanged) {
		t.Fatalf("metadata drift was not rejected: %v", err)
	}
}
