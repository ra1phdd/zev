//go:build windows

package repository

import (
	"os"

	"golang.org/x/sys/windows"
)

const readFlags = 0

func multipleLinks(file *os.File, _ os.FileInfo) bool {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return true
	}
	return info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0
}
