package filestore

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicWriteFailurePreservesPrevious(t *testing.T) {
	t.Parallel()
	failure := errors.New("injected failure")
	for _, stage := range []string{"write", "short write", "sync", "rename"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "data")
			original := []byte("previous complete version")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			w := atomicWriter{write: (*os.File).Write, syncFile: (*os.File).Sync, rename: os.Rename, syncDir: syncDirectory}
			switch stage {
			case "write":
				w.write = func(f *os.File, data []byte) (int, error) {
					n, err := f.Write(data[:1])
					if err != nil {
						return n, fmt.Errorf("injected write: %w", err)
					}
					return n, failure
				}
			case "short write":
				w.write = func(f *os.File, data []byte) (int, error) { return f.Write(data[:1]) }
			case "sync":
				w.syncFile = func(_ *os.File) error { return failure }
			case "rename":
				w.rename = func(_, _ string) error { return failure }
			}
			if err := w.writeFile(path, []byte("replacement"), 0o600); err == nil {
				t.Fatal("write succeeded despite injected failure")
			} else if errors.Is(err, ErrCommitted) {
				t.Fatalf("pre-commit failure classified as committed: %v", err)
			}
			data, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, original) {
				t.Errorf("previous file changed: %q", data)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Errorf("temporary file leaked: %v", entries)
			}
		})
	}
}

func TestAtomicWriteReplacesFilePrivately(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "data")
	if err := AtomicWrite(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { //nolint:gosec // Verify replacement tightens previously public permissions.
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Errorf("data = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode())
	}
}

func TestWithLockConcurrentUpdates(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	const count = 40
	errs := make(chan error, count)
	for range count {
		go func() {
			errs <- WithLock(path, func() error {
				data, err := os.ReadFile(filepath.Clean(path))
				if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("read counter: %w", err)
				}
				return AtomicWrite(path, append(data, 'x'), 0o600)
			})
		}()
	}
	for range count {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != count {
		t.Errorf("got %d updates, want %d", len(data), count)
	}
	failure := errors.New("callback failure")
	if err := WithLock(path, func() error { return failure }); !errors.Is(err, failure) {
		t.Errorf("error = %v", err)
	}
	if err := WithLock(path, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}
