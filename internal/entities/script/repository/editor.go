package repository

import (
	"errors"
	"io"
	"os"
)

// ReadEditorFile reads an editor working file through its already-open root.
func ReadEditorFile(root *os.Root, name string) ([]byte, error) {
	f, err := openRegular(root, name)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(f)
	return data, errors.Join(readErr, f.Close())
}
