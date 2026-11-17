//go:build windows

package contextfile

import (
	"fmt"
	"os"
)

// os.Root confines reparse resolution; opened-handle identity checks enforce
// the no-symlink policy before reading. Unix open flags are not portable here.
func openContextLeaf(root *os.Root, name string) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open context leaf: %w", err)
	}
	return file, nil
}
