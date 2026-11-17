package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

func TestNamedQueryPreservesCorruptAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir, err := config.GetZevDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, "aliases")
	original := []byte("fixture: [broken")
	if err := os.WriteFile(filename, original, 0o600); err != nil {
		t.Fatal(err)
	}
	handled, err := executeNamedQuery(t.Context(), "fixture", slog.New(slog.DiscardHandler))
	if !handled || err == nil {
		t.Fatalf("handled=%v error=%v; corruption must not fall through to provider", handled, err)
	}
	after, err := os.ReadFile(filename) //nolint:gosec // filename is created inside this test's private temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("corrupted aliases were replaced")
	}
}
