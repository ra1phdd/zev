package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

func TestManagedInteractiveGenerationRequiresSeparateConsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var calls atomic.Int32
	var wireBody, wireURL, consentDiagnostic string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		wireBody = string(data)
		wireURL = r.URL.String()
		calls.Add(1)
		if _, err := io.WriteString(w, `{"choices":[{"message":{"content":"{\"description\":\"consented metadata\"}"}}]}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_API_KEY", "fixture")
	t.Setenv("OPENAI_MODEL", "fixture-model")
	old := managedTerminal
	managedTerminal = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { managedTerminal = old })
	for _, tt := range []struct {
		name, answer string
		wantCalls    int32
	}{{"declined", "no", 0}, {"consented", "yes", 1}} {
		root := NewRootCommand("test")
		var out, diagnostic bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&diagnostic)
		root.SetIn(strings.NewReader(tt.name + "\necho\n\n" + tt.answer + "\n"))
		root.SetArgs([]string{managedKindAlias, managedOpAdd, "--generate-description"})
		if err := root.ExecuteContext(t.Context()); err != nil {
			t.Fatal(err)
		}
		if calls.Load() != tt.wantCalls {
			t.Fatalf("%s: calls=%d", tt.name, calls.Load())
		}
		if !strings.Contains(diagnostic.String(), server.URL) || !strings.Contains(diagnostic.String(), tt.name) || !strings.Contains(diagnostic.String(), "UTF-8") {
			t.Fatalf("missing disclosure: %s", diagnostic.String())
		}
		if strings.Contains(out.String(), "field=description") {
			t.Fatal("request leaked to stdout")
		}
		if tt.answer == "yes" {
			consentDiagnostic = diagnostic.String()
		}
	}
	if !strings.Contains(wireBody, "entity_name") || wireURL != "/chat/completions" {
		t.Fatalf("wire request = %q at %q", wireBody, wireURL)
	}
	if !strings.Contains(consentDiagnostic, `destination=`+server.URL+`/chat/completions`) {
		t.Fatalf("consent did not disclose final operation URL: %s", consentDiagnostic)
	}
	if !strings.Contains(consentDiagnostic, wireBody) {
		t.Fatalf("consent did not disclose exact serialized body: %s", consentDiagnostic)
	}
	if !strings.Contains(consentDiagnostic, fmt.Sprintf("%d UTF-8", len([]byte(wireBody)))) {
		t.Fatalf("consent did not disclose transmitted byte count: %s", consentDiagnostic)
	}
}

func TestManagedGenerationDoesNotFollowConsentedRequest307Redirect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "fixture")
	t.Setenv("OPENAI_MODEL", "fixture-model")
	var redirected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Location", "/redirected")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		redirected.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	t.Setenv("OPENAI_BASE_URL", server.URL)
	old := managedTerminal
	managedTerminal = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { managedTerminal = old })

	root := NewRootCommand("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader("redirected\necho\n\nyes\n"))
	root.SetArgs([]string{managedKindAlias, managedOpAdd, "--generate-description"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := redirected.Load(); got != 0 {
		t.Fatalf("307 redirect target requests = %d, want 0", got)
	}
}

func TestManagedScriptGenerationReadsBoundedPinnedBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	body := filepath.Join(home, "source")
	if err := os.WriteFile(body, []byte("#!/bin/bash\necho pinned-generation-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if !strings.Contains(string(data), "pinned-generation-source") {
			t.Error("registered body not in request")
		}
		if _, err := io.WriteString(w, `{"choices":[{"message":{"content":"{\"description\":\"generated script metadata\"}"}}]}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_API_KEY", "fixture")
	t.Setenv("OPENAI_MODEL", "fixture-model")
	if _, err := managedCommand(t, managedKindScript, managedOpCreate, "pinned", "--body-file", body, historyJSONFlag); err != nil {
		t.Fatal(err)
	}
	out, err := managedCommand(t, managedKindScript, managedOpEdit, "pinned", "--generate-description", historyJSONFlag)
	if err != nil || !strings.Contains(out, "generated script metadata") {
		t.Fatalf("%s %v", out, err)
	}
	if err := os.WriteFile(body, []byte(strings.Repeat("x", 65537)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := managedCommand(t, managedKindScript, managedOpEdit, "pinned", "--body-file", body, historyJSONFlag); err != nil {
		t.Fatal(err)
	}
	out, err = managedCommand(t, managedKindScript, managedOpEdit, "pinned", "--generate-description", historyJSONFlag)
	if err != nil || !strings.Contains(out, "generated script metadata") {
		t.Fatalf("oversized fallback: %s %v", out, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("oversized body transmitted: calls=%d", calls.Load())
	}
}
