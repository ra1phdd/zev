//go:build !windows

package repository

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"syscall"
)

const (
	noFollowFlags  = syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	directoryFlags = syscall.O_NOFOLLOW | syscall.O_DIRECTORY
)

func multipleLinks(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink != 1
}

func checkOpenedFile(*os.File) error { return nil }

func requireCombinedTransactionSupport() error { return nil }

func syncRootDirectory(root *os.Root) (err error) {
	file, err := root.OpenFile(".", os.O_RDONLY|directoryFlags, 0)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer closeResource(&err, file)
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
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
