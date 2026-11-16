package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileRegularAndMissing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	if _, err := ReadFile(path); !os.IsNotExist(err) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}
	if err := AtomicWrite(path, []byte("complete"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := ReadFile(path)
	if err != nil || string(data) != "complete" {
		t.Fatalf("ReadFile = %q, %v", data, err)
	}
}

func TestTrustedAncestorSymlinkRemainsSupported(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "home-alias")
	if err := os.Symlink(home, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dir := filepath.Join(alias, ".zev")
	if err := ValidateDirectory(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "data")
	if err := WithLock(path, func() error { return AtomicWrite(path, []byte("safe"), 0o600) }); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(dir); err != nil {
		t.Fatal(err)
	}
	data, err := ReadFile(path)
	if err != nil || string(data) != "safe" {
		t.Fatalf("ReadFile through trusted ancestor = %q, %v", data, err)
	}
}

func TestOpenReadFileRejectsSymlinkWithoutPrecheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	victim, path := filepath.Join(dir, "victim"), filepath.Join(dir, "link")
	if err := os.WriteFile(victim, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	file, err := openReadFile(path)
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if err == nil {
		t.Fatal("nofollow open accepted a leaf symlink")
	}
}
