package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWithLockContextCancellation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithLock(path, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	t.Cleanup(func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	t.Cleanup(cancel)
	called := false
	err := WithLockContext(ctx, path, func() error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("error = %v, callback called = %v", err, called)
	}
	ctx, cancelNow := context.WithCancel(t.Context())
	cancelNow()
	if err := WithLockContext(ctx, filepath.Join(t.TempDir(), "unused"), func() error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled acquisition = %v", err)
	}
}

func TestStorageRejectsUnsafeFiles(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"symlink", "hardlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			victim := filepath.Join(dir, "victim")
			if err := os.WriteFile(victim, []byte("unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "data")
			lock := path + ".lock"
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(victim, lock)
			case "hardlink":
				err = os.Link(victim, lock)
			case "directory":
				err = os.Mkdir(lock, 0o700)
			}
			if err != nil {
				t.Skipf("cannot create %s: %v", kind, err)
			}
			called := false
			if err := WithLock(path, func() error { called = true; return nil }); err == nil || called {
				t.Fatalf("unsafe sidecar accepted: error = %v, called = %v", err, called)
			}
			data, err := os.ReadFile(filepath.Clean(victim))
			if err != nil || string(data) != "unchanged" {
				t.Fatalf("victim changed: data = %q, error = %v", data, err)
			}
		})
	}
}

func TestAtomicWriteRejectsSymlinkDestination(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	victim, path := filepath.Join(dir, "victim"), filepath.Join(dir, "data")
	if err := os.WriteFile(victim, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := AtomicWrite(path, []byte("new"), 0o600); err == nil {
		t.Fatal("symlink destination accepted")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: info = %v, error = %v", info, err)
	}
}

func TestAtomicWriteDirectorySyncFailureIsPostCommit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	if err := AtomicWrite(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("directory sync failed")
	w := atomicWriter{
		write:    (*os.File).Write,
		syncFile: (*os.File).Sync,
		rename:   os.Rename,
		syncDir: func(dir string) error {
			if dir != filepath.Dir(path) {
				t.Errorf("sync directory = %q", dir)
			}
			data, err := os.ReadFile(filepath.Clean(path))
			if err != nil || string(data) != "new" {
				t.Errorf("directory sync ran before commit: data = %q, err = %v", data, err)
			}
			return failure
		},
	}
	if err := WithLock(path, func() error { return w.writeFile(path, []byte("new"), 0o600) }); !errors.Is(err, failure) || !errors.Is(err, ErrCommitted) {
		t.Fatalf("post-commit error = %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil || string(data) != "new" {
		t.Fatalf("committed file = %q, error = %v", data, err)
	}
}
