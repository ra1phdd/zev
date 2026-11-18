//go:build !windows

package filetxn

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadRefusesFIFO(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("accepted FIFO")
	}
	if err := Migrate(path, path+".yaml", fixtureConverter); err == nil {
		t.Fatal("migrated FIFO")
	}
}
