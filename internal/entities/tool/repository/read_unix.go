//go:build !windows

package repository

import (
	"os"
	"syscall"
)

const readFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func multipleLinks(_ *os.File, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink != 1
}
