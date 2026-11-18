package service_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

func TestAliasesRejectUnsafeRead(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"symlink", "dangling symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			victim := filepath.Join(dir, "victim")
			if err := os.WriteFile(victim, []byte("version: 2\naliases: {test: {template: echo external}}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "aliases")
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(victim, path)
			case "dangling symlink":
				err = os.Symlink(filepath.Join(dir, "missing"), path)
			case "directory":
				err = os.Mkdir(path, 0o700)
			}
			if err != nil {
				t.Skipf("cannot create %s: %v", kind, err)
			}

			s := service.New(repository.New(dir))
			if _, err := s.List(); err == nil {
				t.Error("List accepted unsafe file")
			}
			if _, err := s.Resolve("test", nil); err == nil || errors.Is(err, service.ErrAliasNotFound) {
				t.Error("Resolve accepted unsafe file")
			}
			if err := s.Add("new", "echo new"); err == nil {
				t.Error("Add accepted unsafe file")
			}

			data, err := os.ReadFile(filepath.Clean(victim))
			if err != nil || string(data) != "version: 2\naliases: {test: {template: echo external}}\n" {
				t.Fatalf("victim changed: %q, %v", data, err)
			}
			info, err := os.Lstat(path)
			if err != nil || info.Mode().IsRegular() {
				t.Fatalf("unsafe leaf replaced: %v, %v", info, err)
			}
		})
	}
}

func TestAliasesRejectsUnsafeDirectory(t *testing.T) {
	t.Parallel()
	for _, dangling := range []bool{false, true} {
		t.Run(fmt.Sprint("dangling=", dangling), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			target := filepath.Join(home, "external")
			if !dangling {
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(home, ".zev")
			if err := os.Symlink(target, dir); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			path := filepath.Join(dir, "aliases")
			r := repository.New(filepath.Dir(path))
			s := service.New(r)
			if _, err := s.List(); err == nil {
				t.Error("List accepted unsafe directory")
			}
			if err := s.Add("test", "echo new"); err == nil {
				t.Error("Add accepted unsafe directory")
			}
			called := false
			if err := r.Update(func(map[string]domain.Alias) error {
				called = true
				return nil
			}); err == nil {
				t.Error("Update accepted unsafe directory")
			}
			if called {
				t.Error("Update called mutation callback for unsafe directory")
			}
			if dangling {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("target created: %v", err)
				}
			} else {
				entries, err := os.ReadDir(target)
				if err != nil || len(entries) != 0 {
					t.Fatalf("external directory modified: %v, %v", entries, err)
				}
			}
		})
	}
}
