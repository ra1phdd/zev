package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestHistoryIndexStatusJSONUsesOptionalIndexPort(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{indexStatus: service.IndexStatus{
		ProfileID:    "lexical-profile",
		Documents:    7,
		LexicalReady: true,
	}}
	var out bytes.Buffer
	cmd := historyTestCommand(fake, &out)
	cmd.SetArgs([]string{historyCommandIndex, "status", historyJSONFlag})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got service.IndexStatus
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Documents != 7 || !got.LexicalReady {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestHistoryIndexVectorRebuildRequiresConsentBeforeNetwork(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{indexStatus: service.IndexStatus{
		EmbeddingCoverage: service.EmbeddingCoverage{Eligible: 3},
	}}
	var out, errOut bytes.Buffer
	cmd := historyTestCommand(fake, &out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{historyCommandIndex, historyCommandRebuild, historyVectorsFlag})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), historyYesFlag) {
		t.Fatalf("rebuild without consent error = %v", err)
	}
	if fake.rebuildVectorsCalls != 0 {
		t.Fatal("provider rebuild started without consent")
	}
	if !strings.Contains(errOut.String(), "embedding-test-model") || !strings.Contains(errOut.String(), "3") {
		t.Fatalf("missing preflight summary: %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
}

func TestHistoryIndexVectorRebuildReportsFailuresAndRetryFlag(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{
		indexStatus:   service.IndexStatus{EmbeddingCoverage: service.EmbeddingCoverage{Eligible: 2}},
		rebuildResult: service.RebuildResult{Ready: 1, Failed: 1},
	}
	var out, errOut bytes.Buffer
	cmd := historyTestCommand(fake, &out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{historyCommandIndex, historyCommandRebuild, historyVectorsFlag, "--retry-failed", historyYesFlag})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.rebuildVectorsCalls != 1 || !fake.retryFailed {
		t.Fatalf("rebuild call = %d retry=%v", fake.rebuildVectorsCalls, fake.retryFailed)
	}
	if !strings.Contains(out.String(), "failed=1") {
		t.Fatalf("missing failure report: %q", out.String())
	}
}

func TestHistoryIndexRebuildModesAreMutuallyExclusive(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{}
	cmd := historyTestCommand(fake, io.Discard)
	cmd.SetArgs([]string{historyCommandIndex, historyCommandRebuild, "--fts", historyVectorsFlag, historyYesFlag})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("both modes error = %v", err)
	}
	if fake.rebuildFTSCalls != 0 || fake.rebuildVectorsCalls != 0 {
		t.Fatal("rebuild started with conflicting modes")
	}
}

func TestOpenHistoryWiresOptInSemanticAdapter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HISTORY_SEMANTIC_ENABLED", "true")
	t.Setenv("HISTORY_EMBEDDING_PROVIDER", "openai-compatible")
	t.Setenv("HISTORY_EMBEDDING_BASE_URL", "http://127.0.0.1:18080/v1")
	t.Setenv("HISTORY_EMBEDDING_MODEL", "test-embedding-model")
	t.Setenv("HISTORY_EMBEDDING_DIMENSIONS", "2")
	t.Setenv("HISTORY_EMBEDDING_MAX_BYTES", "1024")
	if err := os.MkdirAll(filepath.Join(home, ".zev"), 0o700); err != nil {
		t.Fatal(err)
	}
	opened, closer, err := openHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	adapter, ok := opened.(*historyAdapter)
	if !ok || adapter.embedder == nil || adapter.config == nil || !adapter.config.HistorySemanticEnabled {
		t.Fatalf("semantic adapter not wired: %#v", opened)
	}
}

func TestOpenHistoryLeavesSemanticAdapterOptIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HISTORY_SEMANTIC_ENABLED", "false")
	opened, closer, err := openHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	adapter, ok := opened.(*historyAdapter)
	if !ok || adapter.embedder != nil {
		t.Fatalf("semantic adapter unexpectedly enabled: %#v", opened)
	}
}
