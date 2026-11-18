// Package filetxn publishes validated migration results without retaining source backups.
// Callers must hold the normal repository locks throughout conversion and publication.
package filetxn

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

// MaxBytes bounds both configuration formats and all migration reads.
const (
	MaxBytes = 1 << 20
	suffix   = ".migration"
)

// ErrConflict indicates that a file does not belong to the pending migration.
var ErrConflict = errors.New("configuration migration conflict")

// pending inspects the legacy path and journal without creating storage.
func pending(source, dest string) (bool, error) {
	for _, path := range []string{source, dest + suffix} {
		_, err := os.Lstat(filepath.Clean(path))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect migration state: %w", err)
		}
	}
	return false, nil
}

// Migrate publishes the converter's validated output, then removes the verified
// source. Caller must hold both path locks, legacy first. Only hashes and the
// source file identity survive in the journal, not a second copy of credentials.
func Migrate(source, dest string, convert func([]byte) ([]byte, error)) error {
	pending, err := pending(source, dest)
	if err != nil || !pending {
		return err
	}
	return migrate(source, dest, convert, SyncDirectory)
}

func migrate(source, dest string, convert func([]byte) ([]byte, error), syncDir func(string) error) error {
	if filepath.Clean(source) == filepath.Clean(dest) || convert == nil {
		return errors.New("invalid configuration migration parameters")
	}
	markerPath := dest + suffix
	journal, err := readOptional(markerPath, false)
	if err != nil {
		return err
	}
	src, err := readOptional(source, true)
	if err != nil {
		return err
	}
	out, err := readOptional(dest, false)
	if err != nil {
		return err
	}
	if src.id == "" {
		return finishWithoutSource(markerPath, journal, out)
	}
	if out.id != "" {
		if journal.id == "" || !bytes.Equal(journal.data, journalBytes(&src, out.data)) {
			return fmt.Errorf("%w: existing YAML does not match a pending migration", ErrConflict)
		}
		return finishPublication(dest, src, out.data, syncDir)
	}
	output, err := convert(src.data)
	if err != nil {
		return fmt.Errorf("convert legacy configuration: %w", err)
	}
	if len(output) > MaxBytes {
		return errors.New("converted configuration exceeds size limit")
	}
	if err := src.verifySource(); err != nil {
		return err
	}
	expected := journalBytes(&src, output)
	if journal.id != "" && !bytes.Equal(journal.data, expected) {
		return fmt.Errorf("%w: source or generated configuration changed", ErrConflict)
	}
	if journal.id == "" {
		if err := filestore.AtomicWrite(markerPath, expected, 0o600); err != nil {
			return fmt.Errorf("prepare migration: %w", err)
		}
	}
	if err := publish(dest, output); err != nil {
		return err
	}
	return finishPublication(dest, src, output, syncDir)
}

func finishPublication(dest string, src Snapshot, output []byte, syncDir func(string) error) error {
	if err := verify(dest, output); err != nil {
		return err
	}
	// Retry the directory barrier even on recovery: visible != crash durable.
	if err := syncDir(filepath.Dir(dest)); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("sync migrated configuration: %w", err))
	}
	if err := src.verifySource(); err != nil {
		return err
	}
	if err := removeAndSync(src.path); err != nil {
		return err
	}
	return removeAndSync(dest + suffix)
}

func journalBytes(src *Snapshot, output []byte) []byte {
	return fmt.Appendf(nil, "v1 %s %x %x\n", src.id, sha256.Sum256(src.data), sha256.Sum256(output))
}

func validJournal(data, output []byte) bool {
	fields := strings.Fields(string(data))
	return len(fields) == 4 && fields[0] == "v1" && len(fields[2]) == 64 && fields[3] == fmt.Sprintf("%x", sha256.Sum256(output))
}

func verify(path string, want []byte) error {
	got, err := Read(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%w: destination differs from generated YAML", ErrConflict)
	}
	return nil
}

func publish(dest string, output []byte) error {
	stage := dest + suffix + ".stage"
	existing, err := readOptional(stage, false)
	if err != nil {
		return err
	}
	if existing.id == "" {
		if err := filestore.AtomicWrite(stage, output, 0o600); err != nil {
			return fmt.Errorf("stage YAML configuration: %w", err)
		}
	} else if !bytes.Equal(existing.data, output) {
		return fmt.Errorf("%w: staging file already exists", ErrConflict)
	}
	if err := moveExclusive(stage, dest); err != nil {
		return fmt.Errorf("publish YAML without overwriting destination: %w", err)
	}
	return nil
}

func removeAndSync(path string) error {
	if err := os.Remove(filepath.Clean(path)); err != nil {
		return fmt.Errorf("remove migrated configuration artifact: %w", err)
	}
	return SyncDirectory(filepath.Dir(path))
}

func finishWithoutSource(markerPath string, journal, out Snapshot) error {
	if journal.id == "" {
		return nil
	}
	if out.id == "" || !validJournal(journal.data, out.data) {
		return ErrConflict
	}
	// A previous unlink may be visible but not durable. Order it before journal retirement.
	if err := SyncDirectory(filepath.Dir(markerPath)); err != nil {
		return err
	}
	return removeAndSync(markerPath)
}

func (s *Snapshot) verifySource() error {
	current, err := snapshotFile(s.path, false)
	if err != nil {
		return err
	}
	if current.id != s.id || !bytes.Equal(current.data, s.data) {
		return fmt.Errorf("%w: migration source changed", ErrConflict)
	}
	return nil
}
