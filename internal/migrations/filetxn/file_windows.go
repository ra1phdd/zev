package filetxn

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func openFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode config path: %w", err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	return os.NewFile(uintptr(h), path), nil
}

func identity(file *os.File) (string, error) {
	h := windows.Handle(file.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return "", fmt.Errorf("stat config file: %w", err)
	}
	if info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return "", errors.New("expected a single-link regular configuration file")
	}
	return fmt.Sprintf("%x:%x:%x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}

func moveExclusive(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return fmt.Errorf("encode source path: %w", err)
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return fmt.Errorf("encode destination path: %w", err)
	}
	if err := windows.MoveFileEx(a, b, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("move config file: %w", err)
	}
	return nil
}

// SyncDirectory is a no-op on Windows, which has no portable directory fsync.
func SyncDirectory(string) error { return nil }
