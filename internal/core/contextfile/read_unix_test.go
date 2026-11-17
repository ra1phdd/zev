//go:build darwin || linux

package contextfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenContextLeafNeverFollowsInRootSymlink(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootPath, "file.txt"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file.txt", filepath.Join(rootPath, "link.txt")); err != nil {
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
	file, err := openContextLeaf(root, "link.txt")
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if err == nil {
		t.Fatal("leaf open followed an in-root symlink despite O_NOFOLLOW")
	}
}

func TestLoadRejectsFIFO(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadWithOptions(root, []string{"pipe"}, true); err == nil || got != "" {
		t.Fatalf("FIFO = %q, %v; want empty, non-regular-file error", got, err)
	}
}

func TestOpenContextFileRemainsPinnedAcrossParentRename(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootPath, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "dir", "file.txt"), []byte("inside"), 0o600); err != nil {
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
	file, err := openContextFile(root, "dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := os.Rename(filepath.Join(rootPath, "dir"), filepath.Join(rootPath, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(rootPath, "dir")); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len("inside"))
	if _, err := file.Read(data); err != nil || string(data) != "inside" {
		t.Fatalf("pinned file read = %q, %v", data, err)
	}
}
