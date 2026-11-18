package filestore

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// ReadSingleLink reads at most limit bytes from a singly linked regular file.
// The caller must trust the parent directory. The leaf is opened without
// following links and its identity is checked before and after the bounded read.
func ReadSingleLink(path string, limit int64) (_ []byte, err error) {
	if limit < 0 || limit == math.MaxInt64 {
		return nil, errors.New("invalid file size limit")
	}
	path = filepath.Clean(path)
	named, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect storage file: %w", err)
	}
	if !named.Mode().IsRegular() {
		return nil, errors.New("expected a regular storage file")
	}
	f, err := openReadFile(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	opened, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat open storage file: %w", err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(named, opened) {
		return nil, errors.New("storage file changed while opening")
	}
	if err := checkSingleLink(f); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read storage file: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("storage file exceeds size limit")
	}
	named, err = os.Lstat(path)
	if err != nil || !os.SameFile(named, opened) {
		return nil, errors.New("storage file changed while reading")
	}
	return data, nil
}
