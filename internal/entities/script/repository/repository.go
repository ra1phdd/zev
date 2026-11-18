// Package repository persists script metadata and script files.
package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/metadata"
)

const (
	scriptJournalPhasePrepared          = "prepared"
	scriptJournalPhaseBodyPublished     = "body-published"
	scriptJournalPhaseMetadataPublished = "metadata-published"
	scriptJournalPhaseDone              = "done"
	scriptJournalKindCreate             = "create"
	scriptJournalKindReplace            = "replace"
	scriptJournalKindRemove             = "remove"
	journalDirectoryName                = ".scripts-journal"
	journalLeafName                     = "journal"
	maxScriptJournalPayload             = 64 << 10
)

type scriptJournal struct {
	OperationID     string `json:"operationID"` //nolint:tagliatelle // The journal wire format is an established contract.
	Kind            string `json:"kind"`
	RecordID        string `json:"recordID"` //nolint:tagliatelle // The journal wire format is an established contract.
	Phase           string `json:"phase"`
	OldMetadataHash string `json:"oldMetadataHash"`
	NewMetadataHash string `json:"newMetadataHash"`
	OldBodyHash     string `json:"oldBodyHash"`
	NewBodyHash     string `json:"newBodyHash"`
	BodyPath        string `json:"bodyPath"`
	BodyStage       string `json:"bodyStage"`
	BodyBackup      string `json:"bodyBackup"`
	MetadataStage   string `json:"metadataStage"`
	Checksum        string `json:"checksum"`
}

func (j scriptJournal) signed() (scriptJournal, error) {
	j.Checksum = ""
	data, err := json.Marshal(j)
	if err != nil {
		return j, fmt.Errorf("marshal script journal for signing: %w", err)
	}
	hash := sha256.Sum256(data)
	j.Checksum = hex.EncodeToString(hash[:])
	return j, nil
}

func (j scriptJournal) valid() error {
	want := j.Checksum
	signed, err := j.signed()
	if err != nil || want == "" || signed.Checksum != want {
		return errors.New("script journal checksum mismatch")
	}
	if j.OperationID == "" || j.Kind == "" || j.Phase == "" || j.BodyPath == "" || j.MetadataStage == "" || j.NewMetadataHash == "" {
		return errors.New("script journal is incomplete")
	}
	if j.Phase != scriptJournalPhasePrepared && j.Phase != scriptJournalPhaseBodyPublished && j.Phase != scriptJournalPhaseMetadataPublished && j.Phase != scriptJournalPhaseDone {
		return fmt.Errorf("unknown script journal phase %q", j.Phase)
	}
	if (j.Kind == scriptJournalKindCreate || j.Kind == scriptJournalKindReplace) && (j.NewBodyHash == "" || j.BodyStage == "") {
		return errors.New("script journal is missing the new body staging data")
	}
	if j.Kind == scriptJournalKindReplace && (j.RecordID == "" || j.OldBodyHash == "") {
		return errors.New("script journal is missing replacement identity data")
	}
	if j.Kind == scriptJournalKindRemove && j.BodyBackup == "" {
		return errors.New("script journal is missing removal backup data")
	}
	return nil
}

func (st *store) openJournalDirectory(create bool) (*os.Root, error) {
	if st.zev == nil {
		return nil, errors.New("script journal root is unavailable")
	}
	if create {
		if err := st.zev.Mkdir(journalDirectoryName, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create script journal directory: %w", err)
		}
	}
	journal, err := openDirectory(st.zev, journalDirectoryName)
	if err != nil {
		if !create && errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("open script journal directory: %w", err)
	}
	info, err := journal.Stat(".")
	if err != nil {
		discardError(journal.Close())
		return nil, fmt.Errorf("inspect script journal directory: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		discardError(journal.Close())
		return nil, errors.New("script journal directory must be private")
	}
	return journal, nil
}

func (st *store) journalPresent() (bool, error) {
	journal, err := st.openJournalDirectory(false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { discardError(journal.Close()) }()
	_, err = journal.Lstat(journalLeafName)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect script journal presence: %w", err)
	}
	return true, nil
}

func (st *store) readJournalFile() ([]byte, error) {
	journal, err := st.openJournalDirectory(false)
	if err != nil {
		return nil, err
	}
	defer func() { discardError(journal.Close()) }()
	file, err := openRegular(journal, journalLeafName)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxScriptJournalPayload+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("read script journal: %w", err)
	}
	if len(data) > maxScriptJournalPayload {
		return nil, errors.New("script journal exceeds size limit")
	}
	return data, nil
}

func (st *store) writeJournal(j scriptJournal) error {
	signed, err := j.signed()
	if err != nil {
		return fmt.Errorf("sign script journal: %w", err)
	}
	data, err := json.Marshal(signed)
	if err != nil {
		return fmt.Errorf("encode script journal: %w", err)
	}
	if len(data) > maxScriptJournalPayload {
		return errors.New("script journal exceeds size limit")
	}
	journal, err := st.openJournalDirectory(true)
	if err != nil {
		return err
	}
	defer func() { discardError(journal.Close()) }()
	if err := syncRootDirectory(st.zev); err != nil {
		return fmt.Errorf("sync script journal parent: %w", err)
	}
	name, err := randomName()
	if err != nil {
		return fmt.Errorf("name script journal temporary: %w", err)
	}
	temporary := ".journal-" + name
	file, err := journal.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY|noFollowFlags, 0o600)
	if err != nil {
		return fmt.Errorf("create script journal temporary: %w", err)
	}
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	writeErr = errors.Join(writeErr, file.Sync(), file.Close())
	if writeErr != nil {
		discardError(journal.Remove(temporary))
		return fmt.Errorf("write script journal temporary: %w", writeErr)
	}
	if info, statErr := journal.Lstat(journalLeafName); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || multipleLinks(info) {
			discardError(journal.Remove(temporary))
			return errors.New("script journal leaf must be a private regular file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		discardError(journal.Remove(temporary))
		return fmt.Errorf("inspect script journal leaf: %w", statErr)
	}
	if err := journal.Rename(temporary, journalLeafName); err != nil {
		discardError(journal.Remove(temporary))
		return fmt.Errorf("publish script journal: %w", err)
	}
	if err := syncRootDirectory(journal); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("sync script journal directory: %w", err))
	}
	return nil
}

func (st *store) updateJournal(j *scriptJournal, phase string) error {
	j.Phase = phase
	return st.writeJournal(*j)
}

func (st *store) removeJournal() error {
	journal, err := st.openJournalDirectory(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { discardError(journal.Close()) }()
	if err := journal.Remove(journalLeafName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove script journal: %w", err)
	}
	return syncRootDirectory(journal)
}

func hashRootPath(root *os.Root, name string) (string, error) {
	file, err := openRegular(root, name)
	if err != nil {
		return "", fmt.Errorf("open script for hashing: %w", err)
	}
	defer func() { discardError(file.Close()) }()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", fmt.Errorf("read script for hashing: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func optionalHash(root *os.Root, path string) (string, error) {
	rel, err := filepath.Rel(root.Name(), path)
	if err != nil {
		return "", fmt.Errorf("resolve hash path: %w", err)
	}
	hash, err := hashRootPath(root, rel)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return hash, err
}

func discardError(error) {}
func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ErrBodyChanged indicates that a script body no longer matches a pinned snapshot.
var ErrBodyChanged = errors.New("script body changed")

// ErrCommitted indicates that an operation published its replacement before
// reporting a durability error. Callers must not roll back a committed change.
var ErrCommitted = filestore.ErrCommitted

// Repository stores metadata and files beneath an application directory.
// Its parent is trusted; aliases in that parent are canonicalized, but the
// application directory itself, script_files and script leaves cannot be links.
// Cooperative transactions do not constitute a sandbox against hostile same-UID
// processes modifying already-open inodes or the trusted parent.
type Repository struct {
	scriptsYAMLPath string
	scriptsDir      string

	// Replacement hooks keep failure paths testable without changing the public API.
	replaceWrite          func(*os.File, []byte) (int, error)
	replaceSync           func(*os.File) error
	replaceRename         func(*os.Root, string, string) error
	syncMetadataDirectory func(*os.Root) error
	syncBodyDirectory     func(*os.Root, string) error
}

// New constructs a repository without creating files. Only aliases outside
// zevDir are resolved; invalid application paths fail when used.
func New(zevDir string) *Repository {
	if absolute, err := filepath.Abs(zevDir); err == nil {
		if parent, err := filepath.EvalSymlinks(filepath.Dir(absolute)); err == nil {
			zevDir = filepath.Join(parent, filepath.Base(absolute))
		}
	}
	return &Repository{scriptsYAMLPath: filepath.Join(zevDir, "scripts"), scriptsDir: filepath.Join(zevDir, "script_files")}
}

// LoadAll returns validated metadata, or an empty map if it does not exist.
func (r *Repository) LoadAll() (map[string]domain.Script, error) {
	return r.LoadAllContext(context.Background())
}

// LoadAllContext returns validated metadata while honoring ctx before I/O.
func (r *Repository) LoadAllContext(ctx context.Context) (map[string]domain.Script, error) {
	var scripts map[string]domain.Script
	err := r.withStore(ctx, false, func(st *store) error {
		var err error
		scripts, err = st.load()
		return err
	})
	return scripts, err
}

// Create registers a script through a recoverable body/metadata publication journal.
//
//nolint:gocognit // The transaction coordinates staging, journaling, and recovery.
func (r *Repository) Create(ctx context.Context, script domain.Script, content []byte) error {
	return r.withStore(ctx, true, func(st *store) error {
		if err := requireCombinedTransactionSupport(); err != nil {
			return err
		}
		scripts, err := st.load()
		if err != nil {
			return err
		}
		if _, ok := scripts[script.Name]; ok {
			return fmt.Errorf("script %q already exists", script.Name)
		}
		if script.RecordID == "" {
			script.RecordID = uuid.NewString()
		}
		if err := st.validate(script.ScriptFile); err != nil {
			return err
		}
		if _, err := st.files.Lstat(script.ScriptFile); err == nil {
			return fmt.Errorf("script file %q already exists", script.ScriptFile)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect script file %q: %w", script.ScriptFile, err)
		}
		operationID := uuid.NewString()
		bodyStage, bodyHash, err := st.stageBody(operationID, content, 0o700)
		if err != nil {
			return err
		}
		scripts[script.Name] = script
		metadataStage, metadataData, err := st.stageMetadata(scripts)
		if err != nil {
			discardError(removeIfExists(bodyStage))
			return err
		}
		oldMetadataHash, err := optionalHash(st.zev, filepath.Join(st.zev.Name(), "scripts"))
		if err != nil {
			return err
		}
		journal := scriptJournal{OperationID: operationID, Kind: scriptJournalKindCreate, RecordID: script.RecordID, Phase: scriptJournalPhasePrepared, OldMetadataHash: oldMetadataHash, NewMetadataHash: hashBytes(metadataData), NewBodyHash: bodyHash, BodyPath: filepath.Join(st.files.Name(), script.ScriptFile), BodyStage: bodyStage, MetadataStage: metadataStage}
		if err := st.writeJournal(journal); err != nil {
			if !errors.Is(err, ErrCommitted) {
				discardError(removeIfExists(bodyStage))
				discardError(removeIfExists(metadataStage))
			}
			return err
		}
		return st.recoverJournal()
	})
}

func (st *store) stageBody(operationID string, content []byte, mode os.FileMode) (string, string, error) {
	if st.files == nil {
		return "", "", errors.New("script_files is unavailable")
	}
	directory := ".scripts-body-" + operationID
	if err := st.files.Mkdir(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("stage script body: %w", err)
	}
	name := filepath.Join(directory, "body")
	file, err := st.files.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|noFollowFlags, mode)
	if err != nil {
		discardError(st.files.RemoveAll(directory))
		return "", "", fmt.Errorf("create staged script body: %w", err)
	}
	n, writeErr := file.Write(content)
	if writeErr == nil && n != len(content) {
		writeErr = io.ErrShortWrite
	}
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		discardError(st.files.RemoveAll(directory))
		return "", "", fmt.Errorf("write staged script body: %w", err)
	}
	hash, err := hashRootPath(st.files, name)
	if err != nil {
		discardError(st.files.RemoveAll(directory))
		return "", "", fmt.Errorf("hash staged script body: %w", err)
	}
	if err := syncScriptDirectory(st.files, directory); err != nil {
		discardError(st.files.RemoveAll(directory))
		return "", "", fmt.Errorf("sync staged script body: %w", err)
	}
	if err := syncRootDirectory(st.files); err != nil {
		discardError(st.files.RemoveAll(directory))
		return "", "", fmt.Errorf("sync script body parent: %w", err)
	}
	return filepath.Join(st.files.Name(), name), hash, nil
}

// UpdateParams supplies the metadata mutation and optional immutable body prerequisite.
// Change executes under the store lock and must not call back into the repository.
type UpdateParams struct {
	Change func(map[string]domain.Script) error
	Source *BodySnapshot
}

// Update loads the complete script set under the store lock and applies Change.
// Existing record IDs are immutable. Source drift returns ErrBodyChanged without
// publication; ErrCommitted indicates a visible change with uncertain durability.
func (r *Repository) Update(ctx context.Context, params UpdateParams) error {
	return r.withStore(ctx, true, func(st *store) error {
		scripts, err := st.load()
		if err != nil {
			return err
		}
		if params.Source != nil {
			if err := st.validateSnapshot(ctx, scripts, *params.Source); err != nil {
				return err
			}
		}
		if err := applyScriptChange(scripts, params.Change); err != nil {
			return err
		}
		if params.Source != nil {
			if err := st.validateSnapshot(ctx, scripts, *params.Source); err != nil {
				return err
			}
		}
		return st.save(scripts)
	})
}

func applyScriptChange(scripts map[string]domain.Script, change func(map[string]domain.Script) error) error {
	identities := make(map[string]string, len(scripts))
	for name, script := range scripts {
		identities[name] = script.RecordID
	}
	if err := change(scripts); err != nil {
		return err
	}
	for name, recordID := range identities {
		if script, ok := scripts[name]; !ok || script.RecordID != recordID {
			return fmt.Errorf("script %q recordID is immutable", name)
		}
	}
	return nil
}

// Remove unregisters a script and removes its body through the publication journal.
func (r *Repository) Remove(name string) error {
	return r.withStore(context.Background(), true, func(st *store) error {
		if err := requireCombinedTransactionSupport(); err != nil {
			return err
		}
		scripts, err := st.load()
		if err != nil {
			return err
		}
		script, ok := scripts[name]
		if !ok {
			return fmt.Errorf("script %q not found", name)
		}
		if err := st.validate(script.ScriptFile); err != nil {
			return err
		}
		return st.remove(script, scripts)
	})
}

func (st *store) remove(script domain.Script, scripts map[string]domain.Script) error {
	operationID := uuid.NewString()
	if st.files == nil {
		return errors.New("script_files is unavailable")
	}
	backupDir := ".scripts-remove-" + operationID
	if err := st.files.Mkdir(backupDir, 0o700); err != nil {
		return fmt.Errorf("stage script removal: %w", err)
	}
	if err := syncRootDirectory(st.files); err != nil {
		return fmt.Errorf("sync script removal staging parent: %w", err)
	}
	backup := filepath.Join(st.files.Name(), backupDir, "body")
	bodyPath := filepath.Join(st.files.Name(), script.ScriptFile)
	oldBodyHash, err := optionalHash(st.files, bodyPath)
	if err != nil {
		discardError(st.files.RemoveAll(backupDir))
		return err
	}
	delete(scripts, script.Name)
	metadataStage, metadataData, err := st.stageMetadata(scripts)
	if err != nil {
		discardError(st.files.RemoveAll(backupDir))
		return err
	}
	oldMetadataHash, err := optionalHash(st.zev, filepath.Join(st.zev.Name(), "scripts"))
	if err != nil {
		return err
	}
	journal := scriptJournal{OperationID: operationID, Kind: scriptJournalKindRemove, RecordID: script.RecordID, Phase: scriptJournalPhasePrepared, OldMetadataHash: oldMetadataHash, NewMetadataHash: hashBytes(metadataData), OldBodyHash: oldBodyHash, BodyPath: bodyPath, BodyBackup: backup, MetadataStage: metadataStage}
	if err := st.writeJournal(journal); err != nil {
		if !errors.Is(err, ErrCommitted) {
			discardError(removeIfExists(metadataStage))
			discardError(st.files.RemoveAll(backupDir))
		}
		return err
	}
	return st.recoverJournal()
}

func (r *Repository) stageReplacement(st *store, data []byte, mode os.FileMode) (string, error) {
	suffix, err := randomName()
	if err != nil {
		return "", err
	}
	staging := ".replace-" + suffix
	if err := st.files.Mkdir(staging, 0o700); err != nil {
		return "", fmt.Errorf("stage edited script: %w", err)
	}
	stagedName := filepath.Join(staging, "script")
	file, err := st.files.OpenFile(stagedName, os.O_CREATE|os.O_EXCL|os.O_WRONLY|noFollowFlags, mode)
	if err != nil {
		return staging, fmt.Errorf("create staged script: %w", err)
	}
	write := r.replaceWrite
	if write == nil {
		write = (*os.File).Write
	}
	sync := r.replaceSync
	if sync == nil {
		sync = (*os.File).Sync
	}
	n, writeErr := write(file, data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	syncErr := error(nil)
	if writeErr == nil {
		syncErr = sync(file)
	}
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return staging, fmt.Errorf("write staged script: %w", err)
	}
	if err := syncScriptDirectory(st.files, staging); err != nil {
		return staging, fmt.Errorf("sync staged script: %w", err)
	}
	if err := syncRootDirectory(st.files); err != nil {
		return staging, fmt.Errorf("sync script body parent: %w", err)
	}
	return staging, nil
}

// ReplaceParams supplies the immutable source prerequisite and replacement bytes.
type ReplaceParams struct {
	Source BodySnapshot
	Body   []byte
}

// ReplaceScript publishes a replacement only if the record, path, inode and body
// still match Source. Stale edits return ErrBodyChanged before publication.
func (r *Repository) ReplaceScript(ctx context.Context, params ReplaceParams) error {
	return r.withStore(ctx, true, func(st *store) error {
		return r.replaceScript(ctx, st, params)
	})
}

func (r *Repository) replaceScript(ctx context.Context, st *store, params ReplaceParams) error {
	if err := requireCombinedTransactionSupport(); err != nil {
		return err
	}
	scriptFile := params.Source.scriptFile
	scripts, err := st.load()
	if err != nil {
		return err
	}
	if err := st.validateSnapshot(ctx, scripts, params.Source); err != nil {
		return err
	}
	staging, stageErr := r.stageReplacement(st, params.Body, params.Source.info.Mode().Perm())
	if stageErr != nil {
		if staging != "" {
			discardError(st.files.RemoveAll(staging))
		}
		return stageErr
	}
	current, err := st.load()
	if err == nil {
		err = st.validateSnapshot(ctx, current, params.Source)
	}
	if err != nil {
		discardError(st.files.RemoveAll(staging))
		return err
	}
	return r.publishReplacement(st, scripts, scriptFile, staging, params.Source.recordID)
}

func (r *Repository) publishReplacement(st *store, scripts map[string]domain.Script, scriptFile, staging, recordID string) error {
	operationID := uuid.NewString()
	bodyStage := filepath.Join(st.files.Name(), staging, "script")
	metadataStage, metadataData, err := st.stageMetadata(scripts)
	if err != nil {
		discardError(st.files.RemoveAll(staging))
		return err
	}
	oldMetadataHash, err := optionalHash(st.zev, filepath.Join(st.zev.Name(), "scripts"))
	if err != nil {
		return err
	}
	oldBodyHash, err := optionalHash(st.files, filepath.Join(st.files.Name(), scriptFile))
	if err != nil {
		return err
	}
	if oldBodyHash == "" {
		return errors.New("script body disappeared during replacement")
	}
	newBodyHash, err := hashRootPath(st.files, filepath.Join(staging, "script"))
	if err != nil {
		return err
	}
	journal := scriptJournal{OperationID: operationID, Kind: scriptJournalKindReplace, RecordID: recordID, Phase: scriptJournalPhasePrepared, OldMetadataHash: oldMetadataHash, NewMetadataHash: hashBytes(metadataData), OldBodyHash: oldBodyHash, NewBodyHash: newBodyHash, BodyPath: filepath.Join(st.files.Name(), scriptFile), BodyStage: bodyStage, MetadataStage: metadataStage}
	if err := st.writeJournal(journal); err != nil {
		if !errors.Is(err, ErrCommitted) {
			discardError(st.files.RemoveAll(staging))
			discardError(removeIfExists(metadataStage))
		}
		return err
	}
	if err := st.recoverJournal(); err != nil {
		if !errors.Is(err, ErrCommitted) {
			discardError(st.removeJournal())
			discardError(st.files.RemoveAll(staging))
			discardError(removeIfExists(metadataStage))
		}
		return err
	}
	return nil
}

// OpenScriptPinned resolves metadata and opens its body during one locked
// transaction. The callback runs after both have been validated and opened.
func (r *Repository) OpenScriptPinned(ctx context.Context, script domain.Script, callback func(domain.Script) error) (*os.File, error) {
	var file *os.File
	err := r.withStore(ctx, false, func(st *store) error {
		scripts, err := st.load()
		if err != nil {
			return err
		}
		current, ok := scripts[script.Name]
		if !ok || current.RecordID != script.RecordID || current.ScriptFile != script.ScriptFile {
			return ErrBodyChanged
		}
		if err := st.validate(current.ScriptFile); err != nil {
			return err
		}
		file, err = openRegular(st.files, current.ScriptFile)
		if err != nil {
			return err
		}
		if callback != nil {
			if err := callback(current); err != nil {
				closeErr := file.Close()
				file = nil
				return errors.Join(err, closeErr)
			}
		}
		return nil
	})
	if err != nil && file != nil {
		closeResource(&err, file)
		file = nil
	}
	return file, err
}

type store struct {
	zev                   *os.Root
	files                 *os.Root
	staging               string
	syncMetadataDirectory func(*os.Root) error
	syncBodyDirectory     func(*os.Root, string) error
	replaceRename         func(*os.Root, string, string) error
}

func (r *Repository) withStore(ctx context.Context, create bool, fn func(*store) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("access script store: %w", err)
	}
	zevDir := filepath.Dir(r.scriptsYAMLPath)
	// Validate before lock acquisition, which itself creates a persistent sidecar.
	if err := validateFilePath(zevDir, true); err != nil {
		return err
	}
	if err := validateFilePath(r.scriptsYAMLPath, false); err != nil {
		return err
	}
	if err := validateFilePath(r.scriptsDir, true); err != nil {
		return err
	}
	parent := filepath.Dir(zevDir)
	// Keep the lock outside the replaceable application directory. This also gives
	// all instances constructed through trusted-parent aliases the same lock key.
	lockPath := filepath.Join(parent, "."+filepath.Base(zevDir)+"-scripts")
	err := filestore.WithLockContext(ctx, lockPath, func() (err error) {
		p, err := os.OpenRoot(parent)
		if err != nil {
			return fmt.Errorf("open trusted parent: %w", err)
		}
		defer closeResource(&err, p)
		return r.withDirectories(p, create, fn)
	})
	if err != nil {
		return fmt.Errorf("script store transaction: %w", err)
	}
	return nil
}

func (r *Repository) withDirectories(parent *os.Root, create bool, fn func(*store) error) (err error) {
	name := filepath.Base(filepath.Dir(r.scriptsYAMLPath))
	if create {
		if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create application directory: %w", err)
		}
	}
	z, err := openDirectory(parent, name)
	if errors.Is(err, os.ErrNotExist) && !create {
		return fn(&store{})
	}
	if err != nil {
		return fmt.Errorf("open application directory: %w", err)
	}
	defer closeResource(&err, z)
	if create {
		if err := z.Mkdir("script_files", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create scripts directory: %w", err)
		}
	}
	files, err := openDirectory(z, "script_files")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("open scripts directory: %w", err)
	}
	if files != nil {
		defer closeResource(&err, files)
	}
	st := &store{zev: z, files: files, syncMetadataDirectory: r.syncMetadataDirectory, syncBodyDirectory: r.syncBodyDirectory, replaceRename: r.replaceRename}
	if err := st.recoverJournal(); err != nil {
		return err
	}
	if create {
		return st.withStaging(fn)
	}
	return fn(st)
}

func (st *store) withStaging(fn func(*store) error) (err error) {
	suffix, err := randomName()
	if err != nil {
		return err
	}
	st.staging = ".scripts-" + suffix
	if err := st.zev.Mkdir(st.staging, 0o700); err != nil {
		return fmt.Errorf("stage metadata: %w", err)
	}
	defer func() {
		if err != nil {
			present, journalErr := st.journalPresent()
			if present || journalErr != nil {
				err = errors.Join(err, journalErr)
				return
			}
		}
		if cleanupErr := st.zev.RemoveAll(st.staging); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove metadata staging directory: %w", cleanupErr))
		}
	}()
	return fn(st)
}

type journalPaths struct {
	metadataPath  string
	metadataStage string
	bodyPath      string
	stagePath     string
	backupPath    string
}

//nolint:gocognit // Recovery advances explicit journal phases and preserves committed failures.
func (st *store) recoverJournal() (err error) {
	if st.zev == nil {
		return nil
	}
	journal, err := st.readJournal()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := requireCombinedTransactionSupport(); err != nil {
		return err
	}
	defer func() {
		if err != nil && journal.Phase != scriptJournalPhasePrepared {
			err = errors.Join(ErrCommitted, err)
		}
		if errors.Is(err, ErrCommitted) {
			err = errors.Join(err, fmt.Errorf("script operation %s: durability is unknown", journal.OperationID))
		}
	}()
	paths, err := st.resolveJournalPaths(journal)
	if err != nil {
		return err
	}
	if journal.Phase == scriptJournalPhasePrepared {
		if err := st.recoverPrepared(journal, paths); err != nil {
			return err
		}
		if err := st.updateJournal(&journal, scriptJournalPhaseBodyPublished); err != nil {
			return errors.Join(filestore.ErrCommitted, fmt.Errorf("record script body publication: %w", err))
		}
	}
	if journal.Phase == scriptJournalPhaseBodyPublished {
		if err := st.recoverBodyPublished(journal, paths); err != nil {
			return err
		}
		if err := st.updateJournal(&journal, scriptJournalPhaseMetadataPublished); err != nil {
			return errors.Join(filestore.ErrCommitted, fmt.Errorf("record script metadata publication: %w", err))
		}
	}
	if journal.Phase == scriptJournalPhaseMetadataPublished {
		if err := st.syncMetadata(paths); err != nil {
			return errors.Join(ErrCommitted, err)
		}
		if err := st.updateJournal(&journal, scriptJournalPhaseDone); err != nil {
			return errors.Join(ErrCommitted, fmt.Errorf("record script metadata durability: %w", err))
		}
	}
	if journal.Phase == scriptJournalPhaseDone {
		return st.cleanupJournal(paths)
	}
	return nil
}

func (st *store) readJournal() (scriptJournal, error) {
	data, err := st.readJournalFile()
	if err != nil {
		return scriptJournal{}, fmt.Errorf("read script journal: %w", err)
	}
	var journal scriptJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return scriptJournal{}, fmt.Errorf("decode script journal: %w", err)
	}
	if err := journal.valid(); err != nil {
		return scriptJournal{}, fmt.Errorf("validate script journal: %w", err)
	}
	return journal, nil
}

func (st *store) resolveJournalPaths(journal scriptJournal) (journalPaths, error) {
	metadataStage, err := rootedPath(st.zev, journal.MetadataStage)
	if err != nil {
		return journalPaths{}, err
	}
	bodyPath, err := rootedPath(st.files, journal.BodyPath)
	if err != nil {
		return journalPaths{}, err
	}
	stagePath, err := rootedPath(st.files, journal.BodyStage)
	if journal.BodyStage != "" && err != nil {
		return journalPaths{}, err
	}
	backupPath, err := rootedPath(st.files, journal.BodyBackup)
	if journal.BodyBackup != "" && err != nil {
		return journalPaths{}, err
	}
	return journalPaths{
		metadataPath: filepath.Join(st.zev.Name(), "scripts"), metadataStage: metadataStage,
		bodyPath: bodyPath, stagePath: stagePath, backupPath: backupPath,
	}, nil
}

func (st *store) syncBodyDirectories(sourcePath, destinationPath string) error {
	syncDirectory := st.syncBodyDirectory
	if syncDirectory == nil {
		syncDirectory = syncScriptDirectory
	}
	paths := []string{sourcePath, destinationPath}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		relative, err := filepath.Rel(st.files.Name(), filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("resolve script body directory: %w", err)
		}
		if _, ok := seen[relative]; ok {
			continue
		}
		seen[relative] = struct{}{}
		if err := syncDirectory(st.files, relative); err != nil {
			return errors.Join(ErrCommitted, fmt.Errorf("sync script body directory %q: %w", relative, err))
		}
	}
	return nil
}

func (st *store) syncBodyDirectoryAt(relative string) error {
	syncDirectory := st.syncBodyDirectory
	if syncDirectory == nil {
		syncDirectory = syncScriptDirectory
	}
	if err := syncDirectory(st.files, relative); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("sync script body directory %q: %w", relative, err))
	}
	return nil
}

func (st *store) recoverPrepared(journal scriptJournal, paths journalPaths) error {
	switch journal.Kind {
	case scriptJournalKindCreate:
		return st.recoverPreparedBody(journal, paths, false)
	case scriptJournalKindReplace:
		return st.recoverPreparedBody(journal, paths, true)
	case scriptJournalKindRemove:
		return st.recoverPreparedRemove(journal, paths)
	default:
		return fmt.Errorf("unknown script journal operation %q", journal.Kind)
	}
}

func (st *store) recoverPreparedBody(journal scriptJournal, paths journalPaths, replacing bool) error {
	currentHash, err := optionalHash(st.files, paths.bodyPath)
	if err != nil {
		return fmt.Errorf("inspect script body before recovery: %w", err)
	}
	if currentHash == journal.NewBodyHash {
		return st.syncBodyDirectories(paths.stagePath, paths.bodyPath)
	}
	if replacing {
		if currentHash != journal.OldBodyHash {
			return errors.New("script body changed during replacement recovery")
		}
	} else if currentHash != "" {
		return errors.New("script body appeared during creation recovery")
	}
	stageHash, err := optionalHash(st.files, paths.stagePath)
	if err != nil {
		return fmt.Errorf("inspect staged script body: %w", err)
	}
	if stageHash != journal.NewBodyHash {
		return errors.New("script journal cannot establish new body")
	}
	if err := renameBody(st, paths.stagePath, paths.bodyPath); err != nil {
		if st.replaceRename != nil {
			return fmt.Errorf("publish script body: %w", err)
		}
		return errors.Join(ErrCommitted, fmt.Errorf("publish script body: %w", err))
	}
	return nil
}

func (st *store) recoverPreparedRemove(journal scriptJournal, paths journalPaths) error {
	currentHash, err := optionalHash(st.files, paths.bodyPath)
	if err != nil {
		return fmt.Errorf("inspect script body before removal recovery: %w", err)
	}
	if currentHash == "" {
		if backupHash, backupErr := optionalHash(st.files, paths.backupPath); backupErr != nil {
			return fmt.Errorf("inspect removed script backup during recovery: %w", backupErr)
		} else if backupHash != journal.OldBodyHash {
			return errors.New("script removal backup is missing or changed")
		}
		return st.syncBodyDirectories(paths.bodyPath, paths.backupPath)
	}
	if currentHash != journal.OldBodyHash {
		return errors.New("script body changed during removal recovery")
	}
	if err := renameBody(st, paths.bodyPath, paths.backupPath); err != nil {
		if st.replaceRename != nil {
			return fmt.Errorf("backup removed script body: %w", err)
		}
		return errors.Join(ErrCommitted, fmt.Errorf("backup removed script body: %w", err))
	}
	return nil
}

func (st *store) recoverBodyPublished(journal scriptJournal, paths journalPaths) error {
	if journal.Kind == scriptJournalKindRemove {
		if hash, err := optionalHash(st.files, paths.bodyPath); err != nil || hash != "" {
			return errors.New("removed script body is still present")
		}
		if hash, err := optionalHash(st.files, paths.backupPath); err != nil || hash != journal.OldBodyHash {
			return errors.New("script removal backup is missing or changed")
		}
	} else if hash, err := optionalHash(st.files, paths.bodyPath); err != nil || hash != journal.NewBodyHash {
		return errors.New("published script body hash mismatch")
	}
	currentHash, err := optionalHash(st.zev, paths.metadataPath)
	if err != nil {
		return fmt.Errorf("inspect script metadata during recovery: %w", err)
	}
	if currentHash == journal.NewMetadataHash {
		return st.finishMetadataPublication(journal, paths)
	}
	stageHash, err := optionalHash(st.zev, paths.metadataStage)
	if err != nil {
		return fmt.Errorf("inspect staged script metadata: %w", err)
	}
	if stageHash != journal.NewMetadataHash {
		return errors.New("staged script metadata is missing or changed")
	}
	if currentHash != journal.OldMetadataHash {
		return errors.New("script metadata changed during recovery")
	}
	if err := os.Rename(paths.metadataStage, paths.metadataPath); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("publish script metadata: %w", err))
	}
	return st.finishMetadataPublication(journal, paths)
}

func (st *store) finishMetadataPublication(journal scriptJournal, paths journalPaths) error {
	if err := st.syncMetadata(paths); err != nil {
		journal.Phase = scriptJournalPhaseMetadataPublished
		phaseErr := st.updateJournal(&journal, scriptJournalPhaseMetadataPublished)
		return errors.Join(ErrCommitted, err, phaseErr)
	}
	return nil
}

func (st *store) syncMetadata(paths journalPaths) error {
	rel, err := filepath.Rel(st.zev.Name(), filepath.Dir(paths.metadataStage))
	if err != nil {
		return fmt.Errorf("resolve metadata staging directory: %w", err)
	}
	if err := syncScriptDirectory(st.zev, rel); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrCommitted, fmt.Errorf("sync metadata source directory: %w", err))
	}
	syncDirectory := st.syncMetadataDirectory
	if syncDirectory == nil {
		syncDirectory = syncRootDirectory
	}
	if err := syncDirectory(st.zev); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("sync script metadata directory: %w", err))
	}
	return nil
}

func (st *store) cleanupJournal(paths journalPaths) error {
	if err := removePrivateArtifact(paths.stagePath); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("remove staged script body: %w", err))
	}
	if err := removePrivateArtifact(paths.backupPath); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("remove script body backup: %w", err))
	}
	if err := removePrivateArtifact(paths.metadataStage); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("remove staged script metadata: %w", err))
	}
	if err := st.removeJournal(); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("remove script journal: %w", err))
	}
	return nil
}

func rootedPath(root *os.Root, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if root == nil {
		return "", errors.New("script journal requires script_files")
	}
	base := filepath.Clean(root.Name())
	candidate := filepath.Clean(path)
	rel, err := filepath.Rel(base, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("script journal path escapes script_files")
	}
	return candidate, nil
}

func renameBody(st *store, from, to string) error {
	fromRel, err := filepath.Rel(st.files.Name(), from)
	if err != nil {
		return fmt.Errorf("resolve staged body path: %w", err)
	}
	toRel, err := filepath.Rel(st.files.Name(), to)
	if err != nil {
		return fmt.Errorf("resolve target body path: %w", err)
	}
	if st.replaceRename != nil {
		if err := st.replaceRename(st.files, fromRel, toRel); err != nil {
			return fmt.Errorf("rename script body: %w", err)
		}
	} else if err := st.files.Rename(fromRel, toRel); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("rename script body outcome is uncertain: %w", err))
	}
	if err := st.syncBodyDirectoryAt(filepath.Dir(fromRel)); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("sync source script body directory: %w", err))
	}
	if filepath.Dir(fromRel) != filepath.Dir(toRel) {
		if err := st.syncBodyDirectoryAt(filepath.Dir(toRel)); err != nil {
			return errors.Join(ErrCommitted, fmt.Errorf("sync destination script body directory: %w", err))
		}
	}
	return nil
}

func syncScriptDirectory(root *os.Root, relative string) (err error) {
	if relative == "." {
		return syncRootDirectory(root)
	}
	directory, err := openDirectory(root, relative)
	if err != nil {
		return fmt.Errorf("open script body directory: %w", err)
	}
	defer closeResource(&err, directory)
	return syncRootDirectory(directory)
}

func removeIfExists(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove path %q: %w", path, err)
	}
	return nil
}

func removePrivateArtifact(path string) error {
	if path == "" {
		return nil
	}
	if err := removeIfExists(path); err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if base := filepath.Base(parent); strings.HasPrefix(base, ".scripts-") || strings.HasPrefix(base, ".replace-") {
		if err := os.Remove(parent); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove private artifact directory: %w", err)
		}
	}
	return nil
}

func (st *store) readMetadata() ([]byte, error) {
	file, err := openRegular(st.zev, "scripts")
	if err != nil {
		return nil, fmt.Errorf("open metadata: %w", err)
	}
	data, readErr := metadata.ReadBounded(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	return data, nil
}

func (st *store) load() (map[string]domain.Script, error) {
	data, err := st.readMetadata()
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]domain.Script), nil
	}
	if err != nil {
		return nil, err
	}
	return st.decodeCanonical(data)
}

func (st *store) decodeCanonical(data []byte) (map[string]domain.Script, error) {
	scripts, err := metadata.DecodeScripts(data)
	if err != nil {
		return nil, fmt.Errorf("decode scripts: %w", err)
	}
	for name, script := range scripts {
		if err := st.validate(script.ScriptFile); err != nil {
			return nil, fmt.Errorf("invalid script %q: %w", name, err)
		}
	}
	return scripts, nil
}

func (st *store) stageMetadata(scripts map[string]domain.Script) (string, []byte, error) {
	for name, script := range scripts {
		if err := st.validate(script.ScriptFile); err != nil {
			return "", nil, fmt.Errorf("invalid script %q: %w", name, err)
		}
	}
	if err := checkRegular(st.zev, "scripts"); err != nil {
		return "", nil, fmt.Errorf("validate metadata: %w", err)
	}
	data, err := metadata.EncodeScriptsV2(scripts)
	if err != nil {
		return "", nil, fmt.Errorf("encode scripts: %w", err)
	}
	temporary := filepath.Join(st.staging, "scripts")
	file, err := st.zev.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY|noFollowFlags, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("create staged metadata: %w", err)
	}
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return "", nil, fmt.Errorf("write staged metadata: %w", err)
	}
	if err := syncScriptDirectory(st.zev, st.staging); err != nil {
		return "", nil, fmt.Errorf("sync staged metadata directory: %w", err)
	}
	if err := syncRootDirectory(st.zev); err != nil {
		return "", nil, fmt.Errorf("sync metadata parent: %w", err)
	}
	return filepath.Join(st.zev.Name(), temporary), data, nil
}

func (st *store) save(scripts map[string]domain.Script) error {
	_, _, err := st.stageMetadata(scripts)
	if err != nil {
		return err
	}
	if err := st.zev.Rename(filepath.Join(st.staging, "scripts"), "scripts"); err != nil {
		return fmt.Errorf("commit script metadata: %w", err)
	}
	syncDirectory := st.syncMetadataDirectory
	if syncDirectory == nil {
		syncDirectory = syncRootDirectory
	}
	if err := syncDirectory(st.zev); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("metadata replacement committed but directory sync failed: %w", err))
	}
	return nil
}

// WithLockedFiles provides the metadata path and a confined check for existing
// script bodies under the repository writer lock. The check is valid only for
// the duration of fn; fn must not acquire the same lock again.
func (r *Repository) WithLockedFiles(ctx context.Context, fn func(string, func(string) error) error) error {
	return r.withStore(ctx, false, func(st *store) error {
		return fn(r.scriptsYAMLPath, func(path string) error {
			if err := validScriptFile(path); err != nil {
				return err
			}
			f, err := openRegular(st.files, path)
			if err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("close script body: %w", err)
			}
			return nil
		})
	})
}
