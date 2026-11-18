package repository

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
)

func TestRepositoryRejectsOversizedAliasesWithoutMutation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases")
	payload := []byte("large: " + strings.Repeat("x", (1<<20)+1))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir).LoadAll(); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected size limit error, got %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != int64(len(payload)) {
		t.Fatalf("oversized source changed: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".aliases-migration")); !os.IsNotExist(err) {
		t.Fatalf("oversized source created migration state: %v", err)
	}
}

func TestRepositoryPersistsCanonicalVersionTwoAliases(t *testing.T) {
	t.Parallel()
	repo := New(t.TempDir())
	if err := repo.Update(func(aliases map[string]domain.Alias) error {
		aliases["show"] = domain.Alias{Name: "show", Template: "printf %s {{.arg1}}", Description: "display a value", Arguments: []domain.Argument{{Name: "value", Description: "text", Required: true}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(repo.aliasesPath), "aliases"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 2") || !strings.Contains(string(data), "description: display a value") {
		t.Fatalf("non-canonical aliases: %q", data)
	}
	got, err := metadata.DecodeAliases(data)
	if err != nil || got["show"].Arguments[0].Name != "value" {
		t.Fatalf("decoded=%#v err=%v", got, err)
	}
}

func TestRepositoryRefusesUnversionedAliases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases")
	source := []byte("show: echo {{.arg1}}\n")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	r := New(dir)
	if _, err := r.LoadAll(); err == nil {
		t.Fatal("unversioned read succeeded")
	}
	if err := r.Update(func(map[string]domain.Alias) error { t.Fatal("mutation callback ran"); return nil }); err == nil {
		t.Fatal("unversioned update succeeded")
	}
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil || !bytes.Equal(source, got) {
		t.Fatalf("source changed: %q, %v", got, err)
	}
}
