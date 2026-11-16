package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

func TestCrossInstanceCreateRemove(t *testing.T) {
	t.Setenv("SKIP_EDITOR", "1")
	_, dir := newTestService(t)
	const count = 24
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Go(func() { errs <- New(repository.New(dir)).Create(fmt.Sprintf("script%d", i), "test") })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	svc := New(repository.New(dir))
	scripts, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != count {
		t.Fatalf("lost creates: got %d want %d", len(scripts), count)
	}
	errs = make(chan error, count)
	for i := range count {
		wg.Go(func() { errs <- New(repository.New(dir)).Remove(fmt.Sprintf("script%d", i)) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	scripts, err = svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 0 {
		t.Fatalf("lost removals: %d remain", len(scripts))
	}
}

func TestCorruptMetadataPreserved(t *testing.T) {
	t.Setenv("SKIP_EDITOR", "1")
	for _, operation := range []string{createOperation, removeOperation} {
		t.Run(operation, func(t *testing.T) {
			svc, dir := newTestService(t)
			if err := svc.Create("safe", "test"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "scripts")
			original := []byte("safe: [broken")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			var err error
			if operation == createOperation {
				err = svc.Create("new", "test")
			} else {
				err = svc.Remove("safe")
			}
			if err == nil {
				t.Fatal("accepted corrupt metadata")
			}
			data, err := os.ReadFile(filepath.Clean(path))
			if err != nil || string(data) != string(original) {
				t.Fatalf("metadata changed: %q %v", data, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "script_files", "safe.sh")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "script_files", "new.sh")); !os.IsNotExist(err) {
				t.Fatal("created file despite corruption")
			}
		})
	}
}
