package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/core/metadatastore"
)

func TestRepositoryRejectsOversizedRecoveryMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "aliases")
	journal := filepath.Join(dir, ".aliases-migration")
	source := []byte("show: echo {{.arg1}}\n")
	canonical := []byte("version: 2\naliases: {}\n")
	if err := os.WriteFile(active, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := metadatastore.Migrate(active, journal, source, canonical); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(journal, "marker")
	marker, err := os.ReadFile(markerPath) //nolint:gosec // The path is created inside this test's private temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	marker = append(marker, []byte(strings.Repeat(" ", (1<<20)-len(marker)+1))...)
	if err := os.WriteFile(markerPath, marker, 0o600); err != nil { //nolint:gosec // The path is created inside this test's private temporary directory.
		t.Fatal(err)
	}
	if _, err := New(dir).LoadAll(); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected recovery marker size limit error, got %v", err)
	}
}

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
	got, legacy, err := metadata.DecodeAliases(data)
	if err != nil || legacy || got["show"].Arguments[0].Name != "value" {
		t.Fatalf("decoded=%#v legacy=%v err=%v", got, legacy, err)
	}
}

func TestRepositoryMigratesLegacyBeforeMutation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases")
	if err := os.WriteFile(path, []byte("show: echo {{.arg1}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := New(dir)
	if err := repo.Update(func(aliases map[string]domain.Alias) error {
		aliases["new"] = domain.Alias{Name: "new", Template: "echo"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	aliases, err := repo.LoadAll()
	if err != nil || aliases["show"].Template != "echo {{.arg1}}" || aliases["new"].Template != "echo" {
		t.Fatalf("migrated aliases=%#v err=%v", aliases, err)
	}
}

func TestRepositoryMigratesLegacyOnReadAndArchivesExactSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases")
	original := []byte("show: echo {{.arg1}}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	aliases, err := New(dir).LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if aliases["show"].Template != "echo {{.arg1}}" {
		t.Fatalf("aliases = %#v", aliases)
	}
	data, err := os.ReadFile(path) //nolint:gosec // The path is created inside this test's private temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == string(original) || !strings.Contains(string(data), "version: 2") {
		t.Fatalf("active file was not migrated: %q", data)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".aliases-migration"))
	if err != nil {
		t.Fatal(err)
	}
	foundArchive := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".archive") {
			archive, readErr := os.ReadFile(filepath.Join(dir, ".aliases-migration", entry.Name())) //nolint:gosec // Entry names come from ReadDir in the private test directory.
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(archive) == string(original) {
				foundArchive = true
			}
		}
	}
	if !foundArchive {
		t.Fatalf("legacy archive not found in %v", entries)
	}
}
