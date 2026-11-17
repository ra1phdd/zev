package contextfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoadRejectsMissingFileAndDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := LoadWithOptions(root, []string{"missing.txt"}, false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error = %v, want os.ErrNotExist", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithOptions(root, []string{"dir"}, false); err == nil {
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
	got, err := LoadWithOptions(root, []string{contextFileA, contextFileB}, false)
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
	if _, err := LoadWithOptions(root, []string{"large.txt"}, false); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("large error = %v, want ErrFileTooLarge", err)
	}
	for _, name := range []string{".env", "server.pem", "credentials.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadWithOptions(root, []string{name}, false); !errors.Is(err, ErrSensitiveFile) {
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
	if _, err := LoadWithOptions(root, []string{"link.txt"}, false); !errors.Is(err, ErrSymlink) {
		t.Fatalf("symlink error = %v, want ErrSymlink", err)
	}
}

func TestLoadRejectsSensitivePathComponents(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		".aws/config", ".docker/config.json", "nested/.ssh/config", ".kube/config",
		".azure/config", ".gnupg/options", ".zev/config", "credentials/readme.txt",
		"nested/.env.local/readme.txt", "private.key/readme.txt", "nested/.AWS/config",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			const content = "synthetic fixture, not credentials"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := LoadWithOptions(root, []string{name}, false); !errors.Is(err, ErrSensitiveFile) || got != "" {
				t.Fatalf("Load() = %q, %v; want empty, ErrSensitiveFile", got, err)
			}
			got, err := LoadWithOptions(root, []string{name}, true)
			if err != nil || !strings.Contains(got, content) {
				t.Fatalf("explicit sensitive permission = %q, %v", got, err)
			}
		})
	}
}

func TestLoadRejectsSymlinkRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, allowSensitive := range []bool{false, true} {
		if got, err := LoadWithOptions(alias, []string{"file.txt"}, allowSensitive); !errors.Is(err, ErrSymlink) || got != "" {
			t.Fatalf("symlink root = %q, %v; want empty, ErrSymlink", got, err)
		}
	}
}

func TestLoadComponentReplacementNeverReadsOutside(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	const sentinel = "OUTSIDE-SYNTHETIC-SENTINEL"
	if err := os.WriteFile(filepath.Join(outside, "file.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "dir")
	parked := filepath.Join(root, "parked")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	workerErr := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := os.Rename(dir, parked); err != nil {
				workerErr <- err
				return
			}
			if err := os.Symlink(outside, dir); err != nil {
				workerErr <- err
				return
			}
			if err := os.Remove(dir); err != nil {
				workerErr <- err
				return
			}
			if err := os.Rename(parked, dir); err != nil {
				workerErr <- err
				return
			}
		}
	})
	t.Cleanup(func() {
		close(stop)
		wg.Wait()
		select {
		case err := <-workerErr:
			t.Error(err)
		default:
		}
	})
	for range 5000 {
		got, err := LoadWithOptions(root, []string{"dir/file.txt"}, true)
		if strings.Contains(got, sentinel) || (err == nil && !strings.Contains(got, "inside")) {
			t.Fatalf("component replacement leaked outside data: %q, %v", got, err)
		}
	}
}

func TestLoadGrowthNeverExceedsRemainingTotal(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	file, err := os.CreateTemp(rootPath, "growing-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := file.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	stop := make(chan struct{})
	workerErr := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := file.Truncate(2); err != nil {
				workerErr <- err
				return
			}
			if err := file.Truncate(1); err != nil {
				workerErr <- err
				return
			}
		}
	})
	t.Cleanup(func() {
		close(stop)
		wg.Wait()
		select {
		case err := <-workerErr:
			t.Error(err)
		default:
		}
	})
	l := loader{root: root, rootPath: rootPath}
	for range 5000 {
		data, _, err := l.loadOne(filepath.Base(file.Name()), MaxTotalBytes-1)
		if err == nil && len(data) > 1 {
			t.Fatalf("read %d bytes with only one byte of aggregate budget", len(data))
		}
		if err != nil && !errors.Is(err, ErrTotalTooLarge) {
			t.Fatalf("unexpected growth error: %v", err)
		}
	}
}

func TestLoadSensitivePermissionDoesNotAllowSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested", "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "real", "file.txt"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		"leaf.txt": "nested/real/file.txt",
		"dir":      "nested/real",
	} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"leaf.txt", "dir/file.txt"} {
		for _, allowSensitive := range []bool{false, true} {
			got, err := LoadWithOptions(root, []string{path}, allowSensitive)
			if !errors.Is(err, ErrSymlink) || got != "" {
				t.Fatalf("LoadWithOptions(%q, %t) = %q, %v; want empty, ErrSymlink", path, allowSensitive, got, err)
			}
		}
	}
}

func TestLoadAllowsTrustedRootAncestorAlias(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "project"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "project", "file.txt"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "trusted-parent")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	got, err := LoadWithOptions(filepath.Join(alias, "project"), []string{"file.txt"}, false)
	if err != nil || !strings.Contains(got, "synthetic") {
		t.Fatalf("trusted ancestor alias = %q, %v", got, err)
	}
}

func TestLoadRejectsOutsideRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, path := range []string{"../outside.txt", filepath.Join(t.TempDir(), "file.txt")} {
		if got, err := LoadWithOptions(root, []string{path}, false); !errors.Is(err, ErrOutsideRoot) || got != "" {
			t.Fatalf("Load(%q) = %q, %v; want empty, ErrOutsideRoot", path, got, err)
		}
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
	paths := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}
	if got, err := LoadWithOptions(root, paths[:4], false); err != nil || got == "" {
		t.Fatalf("exact per-file and total limits = %q, %v; want success", got, err)
	}
	if err := os.WriteFile(filepath.Join(root, "e.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithOptions(root, paths, false); !errors.Is(err, ErrTotalTooLarge) {
		t.Fatalf("total error = %v, want ErrTotalTooLarge", err)
	}
}
