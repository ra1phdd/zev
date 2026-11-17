package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckReadableDoesNotCreateHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.db")
	if err := CheckReadable(context.Background(), path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CheckReadable error = %v, want os.ErrNotExist", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("history database stat error = %v, want os.ErrNotExist", err)
	}
}

func TestCheckReadableAcceptsCurrentHistory(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "history")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := CheckReadable(context.Background(), path); err != nil {
		t.Fatalf("CheckReadable error = %v", err)
	}
}
