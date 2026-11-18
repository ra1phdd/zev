package filestore

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func checkSingleLink(f *os.File) error {
	var st windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &st); err != nil {
		return fmt.Errorf("inspect storage file links: %w", err)
	}
	if st.NumberOfLinks != 1 {
		return errors.New("storage file must have one hard link")
	}
	return nil
}
