package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	historyrepo "github.com/ra1phdd/zev/internal/entities/history/repository"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/spf13/cobra"
)

func TestContinueMachineRestoresFullDialogue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "fixture-only-key")
	t.Setenv("OPENAI_MODEL", "fixture-model")
	t.Setenv("HISTORY_ENABLED", "true")
	t.Setenv("CONTEXT_ENABLED", "false")
	t.Setenv("HTTP_HEADERS", "")
	var requests [][]struct{ Role, Content string }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		requests = append(requests, payload.Messages)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"is_valid\":true,\"commands\":[{\"command\":\"echo repaired\"}]}"}}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("OPENAI_BASE_URL", server.URL)
	store, err := historyrepo.Open(t.Context(), filepath.Join(home, ".zev", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := historyservice.NewLifecycle(store)
	if err := lifecycle.Start(t.Context(), "original request", "fixture", "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Refine(t.Context(), "previous clarification"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Complete(t.Context(), "failed"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{continueQueryFlag, machinePrintFlag, "first followup"}, {continueQueryFlag, historyJSONFlag, "second followup"}} {
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		var out, stderr bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		if err := runQuery(cmd, args); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "Fetching") {
			t.Fatalf("progress in stdout: %q", out.String())
		}
		if args[1] == machinePrintFlag && out.String() != "echo repaired\n" {
			t.Fatalf("stdout = %q", out.String())
		}
		if args[1] == historyJSONFlag && !json.Valid(out.Bytes()) {
			t.Fatalf("invalid stdout JSON: %q", out.String())
		}
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	var joined strings.Builder
	for _, message := range requests[1] {
		joined.WriteString(message.Content)
	}
	for _, want := range []string{"original request", "previous clarification", "first followup", "echo repaired", "second followup"} {
		if !strings.Contains(joined.String(), want) {
			t.Errorf("second continuation missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "repaired")); !os.IsNotExist(err) {
		t.Fatal("unexpected command side effect")
	}
}
