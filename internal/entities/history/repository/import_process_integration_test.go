//go:build integration

package repository

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

const importProcessHelperEnv = "ZEV_IMPORT_PROCESS_HELPER"

func TestImportIntegrationConcurrentOpenWriters(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	if err := os.WriteFile(filepath.Join(dir, "history"), legacyTestLine(t), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	commands := make([]*exec.Cmd, 2)
	outputs := make([]bytes.Buffer, 2)
	for i := range commands {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestImportIntegrationProcessHelper$", "-test.v")
		cmd.Stdout = &outputs[i]
		cmd.Stderr = &outputs[i]
		cmd.Env = append(os.Environ(),
			importProcessHelperEnv+"=1",
			"ZEV_IMPORT_DB="+path,
			"ZEV_IMPORT_WRITER_ID="+string(rune('a'+i)),
		)
		commands[i] = cmd
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("concurrent opener %d failed: %v\n%s", i, err, outputs[i].String())
		}
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeImportTest(t, store)
	importTestCount(t, store, `SELECT count(*) FROM conversations`, 3)
	importTestCount(t, store, `SELECT count(*) FROM migration_runs WHERE status='completed'`, 1)
	if _, err := os.Stat(filepath.Join(dir, "history")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy source remained active: %v", err)
	}
}

func TestImportIntegrationProcessHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv(importProcessHelperEnv) != "1" {
		return
	}
	store, err := Open(t.Context(), os.Getenv("ZEV_IMPORT_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeImportTest(t, store)
	id := os.Getenv("ZEV_IMPORT_WRITER_ID")
	if strings.TrimSpace(id) == "" {
		t.Fatal("writer ID is required")
	}
	_, err = store.StartConversation(t.Context(), service.StartConversationInput{
		Conversation: service.Conversation{ID: "writer-" + id},
		InitialMessage: service.Message{
			ID: "writer-message-" + id, Role: "user", Kind: "initial_prompt", Content: "concurrent writer " + id,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
