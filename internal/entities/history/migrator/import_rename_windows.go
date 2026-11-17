//go:build windows

package migrator

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func renameExclusive(oldPath, newPath string) error {
	from, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return fmt.Errorf("encode source path: %w", err)
	}
	to, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return fmt.Errorf("encode archive path: %w", err)
	}
	// MoveFileEx without REPLACE_EXISTING fails if the destination exists.
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("rename archive exclusively: %w", err)
	}
	return nil
}

func openLegacySource(path string) (*os.File, error) {
	namedInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect legacy source: %w", err)
	}
	if err := validateLegacyFileInfo(namedInfo); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode source path: %w", err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(h), path)
	info, statErr := file.Stat()
	if statErr != nil {
		return nil, errors.Join(fmt.Errorf("stat legacy source: %w", statErr), file.Close())
	}
	if !os.SameFile(namedInfo, info) {
		return nil, errors.Join(errors.New("legacy source changed during open"), file.Close())
	}
	if err := validateOpenedLegacyFile(file, info); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}
