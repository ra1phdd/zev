package contextfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsMissingFileAndDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := Load(root, []string{"missing.txt"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error = %v, want os.ErrNotExist", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, []string{"dir"}); err == nil {
		t.Fatal("directory was accepted")
	}
}

func TestLoadSupportsMultipleFilesWithSections(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const (
		contextFileA = "a.txt"
		contextFileB = "b.txt"
	)
	for name, content := range map[string]string{contextFileA: "alpha", contextFileB: "beta"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Load(root, []string{contextFileA, contextFileB})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PROJECT_CONTEXT", "a.txt", "alpha", "b.txt", "beta"} {
		if !strings.Contains(got, want) {
			t.Fatalf("context %q does not contain %q", got, want)
		}
	}
}

func TestLoadRejectsLimitsSensitiveFilesAndSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	large := strings.Repeat("x", MaxFileBytes+1)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(large), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, []string{"large.txt"}); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("large error = %v, want ErrFileTooLarge", err)
	}
	for _, name := range []string{".env", "server.pem", "credentials.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root, []string{name}); !errors.Is(err, ErrSensitiveFile) {
			t.Fatalf("%s error = %v, want ErrSensitiveFile", name, err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, []string{"link.txt"}); !errors.Is(err, ErrSymlink) {
		t.Fatalf("symlink error = %v, want ErrSymlink", err)
	}
}

func TestLoadRejectsTotalLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := range 4 {
		name := filepath.Join(root, string(rune('a'+i))+".txt")
		if err := os.WriteFile(name, []byte(strings.Repeat("x", MaxFileBytes)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "e.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}); !errors.Is(err, ErrTotalTooLarge) {
		t.Fatalf("total error = %v, want ErrTotalTooLarge", err)
	}
}
