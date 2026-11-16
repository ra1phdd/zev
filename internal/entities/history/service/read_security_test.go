package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryRejectsUnsafeRead(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"symlink", "dangling symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			victim := filepath.Join(dir, "victim")
			if err := os.WriteFile(victim, []byte("{\"prompt\":\"external\"}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "history")
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

			s := &Service{historyPath: path}
			if _, err := s.LoadAll(); err == nil {
				t.Error("LoadAll accepted unsafe file")
			}
			if err := s.Save(HistoryEntry{Prompt: "incoming entry"}); err == nil {
				t.Error("Save accepted unsafe file")
			}

			data, err := os.ReadFile(filepath.Clean(victim))
			if err != nil || string(data) != "{\"prompt\":\"external\"}\n" {
				t.Fatalf("victim changed: %q, %v", data, err)
			}
			info, err := os.Lstat(path)
			if err != nil || info.Mode().IsRegular() {
				t.Fatalf("unsafe leaf replaced: %v, %v", info, err)
			}
		})
	}
}

func TestHistoryRejectsUnsafeDirectory(t *testing.T) {
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
			path := filepath.Join(dir, "history")
			s := &Service{historyPath: path}
			if _, err := s.LoadAll(); err == nil {
				t.Error("LoadAll accepted unsafe directory")
			}
			if err := s.Save(HistoryEntry{Prompt: "incoming entry"}); err == nil {
				t.Error("Save accepted unsafe directory")
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
