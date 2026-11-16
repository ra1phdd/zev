//go:build windows

package repository

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev/internal/entities/history/service"
	"golang.org/x/sys/windows"
)

func secureHistoryPath(path string, isDir bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect history path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != isDir || !isDir && !info.Mode().IsRegular() {
		return fmt.Errorf("%w: unexpected history path type", service.ErrInvalid)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("read history owner token: %w", err)
	}
	flags := ""
	if isDir {
		// New SQLite sidecars inherit the private application-directory DACL.
		flags = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + user.User.Sid.String() + ")(A;" + flags + ";FA;;;SY)")
	if err != nil {
		return fmt.Errorf("create history security descriptor: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read history DACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("set private history DACL: %w", err)
	}
	return nil
}
