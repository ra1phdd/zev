//go:build windows

package repository

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOpenRestrictsWindowsHistoryDACLs(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "zev")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.db")
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := openTestStore(t.Context(), t, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeImportTest(t, s) })
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{dir, path, path + "-wal", path + "-shm"} {
		flags := ""
		if name == dir {
			flags = "OICI"
		}
		want, descriptorErr := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + user.User.Sid.String() + ")(A;" + flags + ";FA;;;SY)")
		if descriptorErr != nil {
			t.Fatal(descriptorErr)
		}
		got, descriptorErr := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if descriptorErr != nil {
			t.Fatal(descriptorErr)
		}
		if got == nil || got.String() != want.String() {
			t.Errorf("%s DACL = %v, want %s", name, got, want)
		}
	}
}
