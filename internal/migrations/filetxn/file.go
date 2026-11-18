package filetxn

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Snapshot records the bytes and identity of a file before conversion.
type Snapshot struct {
	path string
	data []byte
	id   string
}

// ReadSnapshot reads a bounded, single-link regular file without changing its mode.
// Hold the repository lock from this read through Replace.
func ReadSnapshot(path string) (*Snapshot, error) {
	return snapshotFile(path, false)
}

// Bytes returns a copy of the source bytes for conversion.
func (s *Snapshot) Bytes() []byte { return bytes.Clone(s.data) }

// Read returns bounded bytes from a regular, single-link file without changing its mode.
func Read(path string) ([]byte, error) {
	s, err := snapshotFile(path, false)
	if err != nil {
		return nil, err
	}
	return s.data, nil
}

func readOptional(path string, repair bool) (Snapshot, error) {
	s, err := snapshotFile(path, repair)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	return *s, nil
}

func snapshotFile(path string, repair bool) (_ *Snapshot, err error) {
	named, err := os.Lstat(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("inspect configuration: %w", err)
	}
	if !named.Mode().IsRegular() {
		return nil, errors.New("expected a regular configuration file")
	}
	file, err := openFile(path)
	if err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat configuration descriptor: %w", err)
	}
	if !os.SameFile(opened, named) || !opened.Mode().IsRegular() {
		return nil, ErrConflict
	}
	id, err := identity(file)
	if err != nil {
		return nil, err
	}
	if repair && runtime.GOOS != "windows" && opened.Mode().Perm() != 0o600 {
		if err := file.Chmod(0o600); err != nil {
			return nil, fmt.Errorf("repair legacy config mode: %w", err)
		}
		checked, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("verify legacy config mode: %w", err)
		}
		if checked.Mode().Perm() != 0o600 {
			return nil, errors.New("legacy config permission repair failed")
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read configuration bytes: %w", err)
	}
	if len(data) > MaxBytes {
		return nil, errors.New("configuration exceeds size limit")
	}
	named, err = os.Lstat(filepath.Clean(path))
	if err != nil || !os.SameFile(opened, named) {
		return nil, ErrConflict
	}
	return &Snapshot{path: path, data: data, id: id}, nil
}
