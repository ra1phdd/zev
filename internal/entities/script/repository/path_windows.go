//go:build windows

package repository

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const (
	noFollowFlags  = 0
	directoryFlags = 0
)

// requireCombinedTransactionSupport gates operations whose body and metadata
// replacements need durable directory synchronization.
func requireCombinedTransactionSupport() error {
	return errors.New("combined script transactions require durable directory synchronization, unavailable on Windows")
}

// Windows has no portable directory flush through os.File.Sync; file contents
// are synced, but a successful rename does not promise power-loss durability.
func syncRootDirectory(_ *os.Root) error { return nil }

// Windows FileInfo omits link counts; validate the actual handle after opening.
func multipleLinks(_ os.FileInfo) bool { return false }

func checkOpenedFile(file *os.File) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return fmt.Errorf("stat file handle: %w", err)
	}
	if info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return errors.New("script must be a singly linked regular file without reparse points")
	}
	return nil
}

func randomName() (string, error) {
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("random staging name: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
