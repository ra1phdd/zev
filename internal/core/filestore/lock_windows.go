package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func openReadFile(path string) (_ *os.File, err error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode storage path: %w", err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(handle), path)
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return nil, fmt.Errorf("stat storage file: %w", err)
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errors.New("storage source must be a regular file, not a reparse point")
	}
	kind, err := windows.GetFileType(handle)
	if err != nil {
		return nil, fmt.Errorf("get storage file type: %w", err)
	}
	if kind != windows.FILE_TYPE_DISK {
		return nil, errors.New("storage source must be a disk file")
	}
	return file, nil
}

func openLockFile(path string) (_ *os.File, err error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode sidecar path: %w", err)
	}
	// Excluding FILE_SHARE_DELETE keeps the sidecar identity stable while open.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("open nofollow sidecar: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return nil, fmt.Errorf("stat sidecar: %w", err)
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		return nil, errors.New("lock sidecar must be a singly linked regular file, not a reparse point")
	}
	kind, err := windows.GetFileType(handle)
	if err != nil {
		return nil, fmt.Errorf("get sidecar file type: %w", err)
	}
	if kind != windows.FILE_TYPE_DISK {
		return nil, errors.New("lock sidecar must be a disk file")
	}
	if err := validateLockIdentity(file, path); err != nil {
		return nil, err
	}
	return file, nil
}

func lockFile(ctx context.Context, file *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait for LockFileEx: %w", err)
		}
		err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return fmt.Errorf("LockFileEx: %w", err)
		}
		if err := waitLockRetry(ctx); err != nil {
			return err
		}
	}
}

// Windows does not expose a portable directory fsync through os.File.Sync.
func syncDirectory(_ string) error {
	return nil
}
