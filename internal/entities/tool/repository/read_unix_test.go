//go:build !windows

package repository

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

func TestSpecialFilesAndHardlinksDenied(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(regular, filepath.Join(root, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"fifo", "hardlink", "regular", "."} {
		request := ExecRequest{Command: builtinCat, Args: []string{path}}
		if err := e.CheckReadOnly(request); err == nil {
			t.Errorf("accepted special/multilink path %q", path)
		}
		if result := e.Execute(t.Context(), request, true); !result.Denied {
			t.Fatalf("read special file: %+v", result)
		}
	}
}

func TestConcurrentSymlinkReplacementCannotEscape(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("OUTSIDE"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "document")
	staging := filepath.Join(root, "replacement")
	if err := os.WriteFile(path, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(stop)
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
	go func() {
		for {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			if err := os.Symlink(outside, staging); err != nil {
				done <- err
				return
			}
			if err := os.Rename(staging, path); err != nil {
				done <- err
				return
			}
			if err := os.WriteFile(staging, []byte("inside"), 0o600); err != nil {
				done <- err
				return
			}
			if err := os.Rename(staging, path); err != nil {
				done <- err
				return
			}
		}
	}()
	t.Cleanup(finish)
	for range 200 {
		result := e.Execute(t.Context(), ExecRequest{Command: builtinCat, Args: []string{"document"}}, true)
		if result.Output != "" && result.Output != "inside" {
			t.Fatalf("escaped root during replacement: %+v", result)
		}
	}
	finish()
}
