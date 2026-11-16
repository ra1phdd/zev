package service_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/repository"
	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

func TestConcurrentAddsDoNotLoseAliases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const count = 40
	errs := make(chan error, count)
	start := make(chan struct{})
	for i := range count {
		go func() {
			<-start
			s := service.New(repository.New(dir))
			errs <- s.Add(fmt.Sprintf("alias-%d", i), "echo test")
		}()
	}
	close(start)
	for range count {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	aliases, err := repository.New(dir).LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != count {
		t.Errorf("got %d aliases, want %d", len(aliases), count)
	}
}

func TestConcurrentAddsAndRemovesPreserveAllUpdates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initial := service.New(repository.New(dir))
	const count = 20
	for i := range count {
		if err := initial.Add(fmt.Sprintf("remove-%d", i), "echo old"); err != nil {
			t.Fatalf("seed alias %d: %v", i, err)
		}
	}

	errs := make(chan error, count*2)
	start := make(chan struct{})
	for i := range count {
		go func() {
			<-start
			s := service.New(repository.New(dir))
			errs <- s.Add(fmt.Sprintf("add-%d", i), "echo new")
		}()
		go func() {
			<-start
			s := service.New(repository.New(dir))
			errs <- s.Remove(fmt.Sprintf("remove-%d", i))
		}()
	}
	close(start)
	for range count * 2 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}

	aliases, err := repository.New(dir).LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != count {
		t.Fatalf("got %d aliases, want %d", len(aliases), count)
	}
	for i := range count {
		if _, ok := aliases[fmt.Sprintf("remove-%d", i)]; ok {
			t.Errorf("removed alias %d is still present", i)
		}
		if _, ok := aliases[fmt.Sprintf("add-%d", i)]; !ok {
			t.Errorf("added alias %d is missing", i)
		}
	}
}

func TestCorruptAliasesAreNotOverwritten(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases")
	original := "broken: [\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.New(repository.New(dir)).Add("new", "echo test"); err == nil {
		t.Fatal("Add accepted corrupt aliases")
	}
	data, err := os.ReadFile(path) //nolint:gosec // Test path is created in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("corrupt aliases overwritten: %q", data)
	}
}
