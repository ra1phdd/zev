//go:build windows

package migrator

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev/internal/entities/history/service"
	"golang.org/x/sys/windows"
)

func secureLegacyFile(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened legacy file: %w", err)
	}
	if err := validateOpenedLegacyFile(file, info); err != nil {
		return err
	}
	return secureLegacyPath(file.Name())
}

func validateLegacyFileInfo(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: unexpected legacy path type", service.ErrInvalid)
	}
	return nil
}

func validateOpenedLegacyFile(file *os.File, info os.FileInfo) error {
	if err := validateLegacyFileInfo(info); err != nil {
		return err
	}
	var handleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &handleInfo); err != nil {
		return fmt.Errorf("inspect legacy file handle: %w", err)
	}
	if handleInfo.NumberOfLinks != 1 || handleInfo.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return fmt.Errorf("%w: legacy file must be a singly linked regular file without reparse points", service.ErrInvalid)
	}
	return nil
}

func secureLegacyPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect legacy path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: unexpected legacy path type", service.ErrInvalid)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("read legacy owner token: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)")
	if err != nil {
		return fmt.Errorf("create legacy security descriptor: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read legacy DACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("set private legacy DACL: %w", err)
	}
	return nil
}
