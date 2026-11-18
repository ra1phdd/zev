// Package metadatastore provides crash-recoverable publication for metadata migrations.
package metadatastore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/filestore"
)

const maxMigrationPayload = 1 << 20

const (
	migrationActiveTmp      = "active.tmp"
	migrationArchive        = "archive.archive"
	migrationPhasePreparing = "preparing"
	migrationPhaseArchived  = "archived"
	migrationPhasePublished = "published"
)

var errInvalidMarker = errors.New("invalid metadata migration marker")

type marker struct {
	OperationID string `json:"operationID"` //nolint:tagliatelle // The journal wire format is an established contract.
	SourceHash  string `json:"sourceHash"`
	ArchiveHash string `json:"archiveHash"`
	ActiveHash  string `json:"activeHash"`
	Archive     string `json:"archive"`
	ArchiveTmp  string `json:"archiveTmp"`
	ActiveTmp   string `json:"activeTmp"`
	Phase       string `json:"phase"`
	Checksum    string `json:"checksum"`
}

func (m marker) signed() (marker, error) {
	m.Checksum = ""
	data, err := json.Marshal(m)
	if err != nil {
		return m, fmt.Errorf("marshal migration marker for signing: %w", err)
	}
	hash := sha256.Sum256(data)
	m.Checksum = hex.EncodeToString(hash[:])
	return m, nil
}

func (m marker) valid() error {
	want := m.Checksum
	signed, err := m.signed()
	if err != nil || want == "" || signed.Checksum != want {
		return errInvalidMarker
	}
	return nil
}

func privatePath(dir, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name || name == "." || name == ".." || name[:1] == string(filepath.Separator) || (len(name) > 3 && name[:3] == ".."+string(filepath.Separator)) {
		return "", errors.New("migration marker path is not private")
	}
	return filepath.Join(dir, name), nil
}

var errMigrationFileLinked = errors.New("migration file has multiple links")

func migrationLinkCount(info os.FileInfo) (uint64, bool) {
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return 0, false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	for _, name := range []string{"Nlink", "NumberOfLinks"} {
		field := value.FieldByName(name)
		if field.IsValid() && field.Kind() >= reflect.Uint && field.Kind() <= reflect.Uint64 {
			return field.Uint(), true
		}
	}
	return 0, false
}

func openMigrationFile(path string) (*os.File, error) {
	return openMigrationFileWithLinks(path, false)
}

func openMigrationFileWithLinks(path string, allowPair bool) (*os.File, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("open migration file parent: %w", err)
	}
	name := filepath.Base(path)
	info, err := root.Lstat(name)
	if err != nil {
		discardError(root.Close())
		return nil, fmt.Errorf("inspect migration file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		discardError(root.Close())
		return nil, errors.New("migration file must be a private regular non-symlink")
	}
	links, linkCountKnown := migrationLinkCount(info)
	if !linkCountKnown || !allowPair && links != 1 || allowPair && links != 2 {
		discardError(root.Close())
		return nil, errMigrationFileLinked
	}
	file, err := root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		discardError(root.Close())
		return nil, fmt.Errorf("open migration file: %w", err)
	}
	opened, statErr := file.Stat()
	if statErr != nil {
		closeErr := file.Close()
		rootErr := root.Close()
		return nil, errors.Join(fmt.Errorf("stat opened migration file: %w", statErr), closeErr, rootErr)
	}
	if !os.SameFile(info, opened) {
		closeErr := file.Close()
		rootErr := root.Close()
		return nil, errors.Join(errors.New("migration file changed while opening"), closeErr, rootErr)
	}
	if err := root.Close(); err != nil {
		return nil, errors.Join(fmt.Errorf("close migration file root: %w", err), file.Close())
	}
	return file, nil
}

func readMigrationFile(path string) ([]byte, error) {
	file, err := openMigrationFile(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxMigrationPayload+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("read migration file: %w", err)
	}
	if len(data) > maxMigrationPayload {
		return nil, errors.New("migration payload exceeds metadata size limit")
	}
	return data, nil
}

// Recover completes or rejects a pending migration journal. It must be called
// while holding the repository's normal lock, before reading the active file.
func Recover(activePath, journalDir string) error {
	if _, err := os.Lstat(journalDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect recovery directory: %w", err)
	}
	if err := ensureJournal(journalDir); err != nil {
		return err
	}
	m, err := readMarker(journalDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return recoverMarker(activePath, journalDir, m)
}

func readMarker(journalDir string) (marker, error) {
	data, err := readMigrationFile(filepath.Join(journalDir, "marker"))
	if err != nil {
		return marker{}, fmt.Errorf("read migration marker: %w", err)
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		return marker{}, fmt.Errorf("decode migration marker: %w", err)
	}
	if m.OperationID == "" || m.SourceHash == "" || m.ArchiveHash == "" || m.ActiveHash == "" || m.Archive == "" || m.Phase == "" {
		return marker{}, fmt.Errorf("%w: incomplete marker", errInvalidMarker)
	}
	if err := m.valid(); err != nil {
		return marker{}, fmt.Errorf("%w: %w", errInvalidMarker, err)
	}
	if m.Phase != migrationPhasePreparing && m.Phase != migrationPhaseArchived && m.Phase != migrationPhasePublished {
		return marker{}, fmt.Errorf("%w: unknown phase %q", errInvalidMarker, m.Phase)
	}
	return m, nil
}

func recoverMarker(activePath, journalDir string, m marker) error {
	return recoverMarkerWithHooks(activePath, journalDir, m, syncDirectory, writeMarker)
}

//nolint:gocognit // Recovery validates journal phases before preserving committed state.
func recoverMarkerWithHooks(activePath, journalDir string, m marker, syncDir func(string) error, write func(string, marker) error) error {
	archivePath, err := privatePath(journalDir, m.Archive)
	if err != nil {
		return err
	}
	var archiveTmp string
	if m.ArchiveTmp != "" {
		archiveTmp, err = privatePath(journalDir, m.ArchiveTmp)
		if err != nil {
			return err
		}
	}
	activeTmp, err := privatePath(journalDir, m.ActiveTmp)
	if err != nil {
		return err
	}
	if err := verifyPublishedArchive(archivePath, archiveTmp, m.ArchiveHash); err != nil {
		if m.Phase == migrationPhasePreparing && errors.Is(err, os.ErrNotExist) {
			return removeStagingWithSync(journalDir, m, syncDir)
		}
		return fmt.Errorf("migration archive: %w", err)
	}
	if m.Phase == migrationPhasePreparing {
		m.Phase = migrationPhaseArchived
		if err := write(journalDir, m); err != nil {
			return err
		}
	}
	activeHash, activeErr := hashFile(activePath)
	if m.Phase == migrationPhasePublished {
		if activeErr != nil {
			return fmt.Errorf("published migration active file: %w", activeErr)
		}
		return removeCompletedMarker(journalDir)
	}
	if activeErr == nil && activeHash == m.ActiveHash {
		if err := syncDir(filepath.Dir(activePath)); err != nil {
			return errors.Join(filestore.ErrCommitted, fmt.Errorf("migration operation %s: active directory sync failed: %w", m.OperationID, err))
		}
		m.Phase = migrationPhasePublished
		if err := write(journalDir, m); err != nil {
			return errors.Join(filestore.ErrCommitted, fmt.Errorf("migration operation %s: publish marker durability is unknown: %w", m.OperationID, err))
		}
		return nil
	}
	return publishRecoveredMetadataWithHooks(activePath, journalDir, activeTmp, m, syncDir, write)
}

func publishRecoveredMetadataWithHooks(activePath, journalDir, activeTmp string, m marker, syncDir func(string) error, write func(string, marker) error) error {
	if err := verifyFileHash(activeTmp, m.ActiveHash); err != nil {
		return fmt.Errorf("migration staged metadata: %w", err)
	}
	sourceHash, err := hashFile(activePath)
	if err != nil {
		return fmt.Errorf("legacy source unavailable during recovery: %w", err)
	}
	if sourceHash != m.SourceHash {
		return errors.New("legacy metadata changed during migration")
	}
	if err := os.Rename(activeTmp, activePath); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("publish migrated metadata outcome is uncertain: %w", err))
	}
	if err := syncDir(filepath.Dir(activePath)); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("migration operation %s: directory sync failed: %w", m.OperationID, err))
	}
	m.Phase = migrationPhasePublished
	if err := write(journalDir, m); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("migration operation %s: publish marker durability is unknown: %w", m.OperationID, err))
	}
	return nil
}

func removeCompletedMarker(journalDir string) error {
	return removeCompletedMarkerWithSync(journalDir, syncDirectory)
}

func removeCompletedMarkerWithSync(journalDir string, syncDir func(string) error) error {
	if err := os.Remove(filepath.Join(journalDir, "marker")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("remove completed migration marker: %w", err))
	}
	if err := syncDir(journalDir); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("sync completed migration journal: %w", err))
	}
	return nil
}

// ClearCompleted removes a completed migration marker after a later canonical
// publication has superseded its active hash.
func ClearCompleted(journalDir string) error {
	path := filepath.Join(journalDir, "marker")
	data, err := readMigrationFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read completed migration marker: %w", err)
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("decode completed migration marker: %w", err)
	}
	if err := m.valid(); err != nil {
		return err
	}
	if m.Phase != migrationPhasePublished {
		return errors.New("migration marker is not complete")
	}
	return removeCompletedMarker(journalDir)
}

// Migrate archives source exactly once and publishes canonical data as a
// recoverable transaction. It returns committed errors after a visible replacement.
func Migrate(activePath, journalDir string, source, canonical []byte) error {
	if err := ensureJournal(journalDir); err != nil {
		return err
	}
	if err := Recover(activePath, journalDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(activePath), 0o700); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	if len(source) > maxMigrationPayload || len(canonical) > maxMigrationPayload {
		return errors.New("migration payload exceeds metadata size limit")
	}
	sourceHash := hashBytes(source)
	activeHash := hashBytes(canonical)
	id := uuid.NewString()
	m := marker{
		OperationID: id, SourceHash: sourceHash, ArchiveHash: sourceHash,
		ActiveHash: activeHash, Archive: "archive-" + sourceHash[:16] + "-" + id + ".archive",
		ArchiveTmp: "archive-" + id + ".tmp", ActiveTmp: "active-" + id + ".tmp", Phase: migrationPhasePreparing,
	}
	if err := writeMarker(journalDir, m); err != nil {
		return err
	}
	archivePath, activeTmp, err := stageMigration(journalDir, m, source, canonical)
	if err != nil {
		return err
	}
	return publishMigration(activePath, journalDir, m, archivePath, activeTmp, source)
}

func stageMigration(journalDir string, m marker, source, canonical []byte) (string, string, error) {
	archiveTmp, err := privatePath(journalDir, m.ArchiveTmp)
	if err != nil {
		return "", "", err
	}
	activeTmp, err := privatePath(journalDir, m.ActiveTmp)
	if err != nil {
		return "", "", err
	}
	if err := writeExclusive(archiveTmp, source); err != nil {
		return "", "", err
	}
	if err := verifyFileHash(archiveTmp, m.SourceHash); err != nil {
		return "", "", err
	}
	if err := writeExclusive(activeTmp, canonical); err != nil {
		return "", "", err
	}
	if err := verifyFileHash(activeTmp, m.ActiveHash); err != nil {
		return "", "", err
	}
	archivePath, err := privatePath(journalDir, m.Archive)
	if err != nil {
		return "", "", err
	}
	return archivePath, activeTmp, nil
}

func publishMigration(activePath, journalDir string, m marker, archivePath, activeTmp string, source []byte) error {
	return publishMigrationWithHooks(activePath, journalDir, m, archivePath, activeTmp, source, writeMarker)
}

func publishMigrationWithHooks(activePath, journalDir string, m marker, archivePath, activeTmp string, source []byte, write func(string, marker) error) error {
	if err := publishArchive(filepath.Join(journalDir, m.ArchiveTmp), archivePath, m.SourceHash); err != nil {
		if errors.Is(err, filestore.ErrCommitted) {
			return errors.Join(err, fmt.Errorf("migration operation %s: durability is unknown", m.OperationID))
		}
		return err
	}
	m.Phase = migrationPhaseArchived
	if err := write(journalDir, m); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("migration operation %s: archive publication marker durability is unknown: %w", m.OperationID, err))
	}
	if err := verifySource(activePath, m.SourceHash, source); err != nil {
		return err
	}
	if err := os.Rename(activeTmp, activePath); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("publish migrated metadata outcome is uncertain: %w", err))
	}
	if err := syncDirectory(filepath.Dir(activePath)); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("migration operation %s: active publication durability is unknown: %w", m.OperationID, err))
	}
	m.Phase = migrationPhasePublished
	if err := write(journalDir, m); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("migration operation %s: publish marker durability is unknown: %w", m.OperationID, err))
	}
	return nil
}

func ensureJournal(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return fmt.Errorf("create migration journal: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect migration journal: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("migration journal must be a private directory")
	}
	return nil
}

func writeMarker(dir string, m marker) error {
	signed, err := m.signed()
	if err != nil {
		return fmt.Errorf("sign migration marker: %w", err)
	}
	data, err := json.Marshal(signed)
	if err != nil {
		return fmt.Errorf("encode migration marker: %w", err)
	}
	path := filepath.Join(dir, "marker")
	if err := filestore.AtomicWrite(path, data, 0o600); err != nil {
		return fmt.Errorf("write migration marker: %w", err)
	}
	return nil
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // Caller controls private journal path.
	if err != nil {
		return fmt.Errorf("create migration staging file: %w", err)
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("write migration staging file: %w", err)
	}
	return nil
}

func publishArchive(tmp, final, expected string) error {
	return publishArchiveWithHooks(tmp, final, expected, syncDirectory, removePath)
}

//nolint:gocognit,nestif // Archive publication distinguishes pre- and post-link failures.
func publishArchiveWithHooks(tmp, final, expected string, syncDir func(string) error, remove func(string) error) error {
	if err := verifyFileHash(final, expected); err == nil {
		if err := remove(tmp); err != nil {
			return errors.Join(filestore.ErrCommitted, fmt.Errorf("remove published archive temporary: %w", err))
		}
		return nil
	} else if errors.Is(err, errMigrationFileLinked) {
		if pairErr := verifyArchivePair(tmp, final, expected); pairErr != nil {
			return fmt.Errorf("archive conflict: %w", pairErr)
		}
		if err := remove(tmp); err != nil {
			return errors.Join(filestore.ErrCommitted, fmt.Errorf("remove published archive temporary: %w", err))
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("archive conflict: %w", err)
	}
	if err := os.Link(tmp, final); err != nil {
		if errors.Is(err, os.ErrExist) {
			verifyErr := verifyArchiveArtifact(tmp, final, expected)
			if errors.Is(verifyErr, os.ErrNotExist) {
				verifyErr = verifyFileHash(tmp, expected)
			}
			if verifyErr == nil {
				if removeErr := remove(tmp); removeErr != nil {
					return errors.Join(filestore.ErrCommitted, fmt.Errorf("remove published archive temporary: %w", removeErr))
				}
				return nil
			}
		}
		return fmt.Errorf("publish migration archive: %w", err)
	}
	if err := syncDir(filepath.Dir(final)); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("archive publication committed but directory sync failed: %w", err))
	}
	if err := remove(tmp); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("remove published archive temporary: %w", err))
	}
	return nil
}

func verifyPublishedArchive(final, tmp, expected string) error {
	if err := verifyFileHash(final, expected); err == nil {
		return nil
	} else if !errors.Is(err, errMigrationFileLinked) {
		return err
	}
	return verifyArchivePair(tmp, final, expected)
}

func verifyArchiveArtifact(tmp, final, expected string) error {
	err := verifyFileHash(final, expected)
	if err == nil {
		return nil
	}
	if errors.Is(err, errMigrationFileLinked) && tmp != "" {
		return verifyArchivePair(tmp, final, expected)
	}
	return err
}

func verifyArchivePair(tmp, final, expected string) error {
	temporary, err := openMigrationFileWithLinks(tmp, true)
	if err != nil {
		return fmt.Errorf("open archive temporary pair: %w", err)
	}
	defer func() { discardError(temporary.Close()) }()
	published, err := openMigrationFileWithLinks(final, true)
	if err != nil {
		return fmt.Errorf("open archive published pair: %w", err)
	}
	defer func() { discardError(published.Close()) }()
	temporaryInfo, err := temporary.Stat()
	if err != nil {
		return fmt.Errorf("stat archive temporary pair: %w", err)
	}
	publishedInfo, err := published.Stat()
	if err != nil {
		return fmt.Errorf("stat archive published pair: %w", err)
	}
	if !os.SameFile(temporaryInfo, publishedInfo) {
		return errors.New("archive temporary pair is not the published inode")
	}
	data, err := io.ReadAll(io.LimitReader(temporary, maxMigrationPayload+1))
	if err != nil {
		return fmt.Errorf("read archive temporary pair: %w", err)
	}
	if len(data) > maxMigrationPayload || hashBytes(data) != expected {
		return errors.New("archive temporary pair hash mismatch")
	}
	return nil
}

func verifySource(path, expected string, source []byte) error {
	data, err := readMigrationFile(path)
	if err != nil {
		return fmt.Errorf("recheck migration source: %w", err)
	}
	if len(data) > maxMigrationPayload || hashBytes(data) != expected || !bytesEqual(data, source) {
		return errors.New("legacy metadata changed during migration")
	}
	return nil
}

func verifyFileHash(path, expected string) error {
	got, err := hashFile(path)
	if err != nil {
		return err
	}
	if got != expected {
		return fmt.Errorf("hash mismatch: got %s", got)
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := openMigrationFile(path)
	if err != nil {
		return "", fmt.Errorf("open file for hashing: %w", err)
	}
	defer func() { discardError(file.Close()) }()
	data, err := io.ReadAll(io.LimitReader(file, maxMigrationPayload+1))
	if err != nil {
		return "", fmt.Errorf("read file for hashing: %w", err)
	}
	if len(data) > maxMigrationPayload {
		return "", errors.New("migration payload exceeds metadata size limit")
	}
	return hashBytes(data), nil
}

func discardError(error) {}
func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func bytesEqual(a, b []byte) bool { return bytes.Equal(a, b) }

func removeStagingWithSync(dir string, m marker, syncDir func(string) error) error {
	archiveTmp, err := privatePath(dir, m.ArchiveTmp)
	if err != nil {
		return err
	}
	activeTmp, err := privatePath(dir, m.ActiveTmp)
	if err != nil {
		return err
	}
	if err := removePath(archiveTmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := removePath(activeTmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(filepath.Join(dir, "marker")); err != nil {
		return fmt.Errorf("remove migration marker: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return errors.Join(filestore.ErrCommitted, fmt.Errorf("sync migration journal after marker removal: %w", err))
	}
	return nil
}

func removePath(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove path %q: %w", path, err)
	}
	return nil
}

func syncDirectory(path string) (err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect directory for syncing: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("migration sync path must be a directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return fmt.Errorf("open directory for syncing: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	file, err := root.OpenFile(".", os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open directory for syncing: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
