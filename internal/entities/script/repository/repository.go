// Package repository persists script metadata and script files.
package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"gopkg.in/yaml.v3"
)

type scriptYAML struct {
	Script      string `yaml:"script"`
	Description string `yaml:"description"`
}

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

// SaveAll atomically replaces validated metadata under the repository lock.
// Read-modify-write callers should use Create or Remove instead.
func (r *Repository) SaveAll(scripts map[string]domain.Script) error {
	return r.withStore(context.Background(), true, func(st *store) error { return st.save(scripts) })
}

// Create registers a script after exclusively creating its file under the store
// lock. Pre-commit metadata failures remove only the file this operation created.
// Creation and removal are not crash-atomic transactions across both files.
func (r *Repository) Create(ctx context.Context, script domain.Script, content []byte) error {
	return r.withStore(ctx, true, func(st *store) (err error) {
		scripts, err := st.load()
		if err != nil {
			return err
		}
		if _, ok := scripts[script.Name]; ok {
			return fmt.Errorf("script %q already exists", script.Name)
		}
		if err := st.validate(script.ScriptFile); err != nil {
			return err
		}
		f, err := st.files.OpenFile(script.ScriptFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollowFlags, 0o700)
		if err != nil {
			return fmt.Errorf("create script: %w", err)
		}
		original, statErr := f.Stat()
		_, writeErr := f.Write(content)
		syncErr := f.Sync()
		closeErr := f.Close()
		committed := false
		defer func() {
			if !committed && statErr == nil {
				err = errors.Join(err, st.rollbackCreated(script.ScriptFile, original))
			}
		}()
		if err := errors.Join(statErr, writeErr, syncErr, closeErr); err != nil {
			return fmt.Errorf("write script: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("create script canceled: %w", err)
		}
		scripts[script.Name] = script
		err = st.save(scripts)
		committed = err == nil || errors.Is(err, ErrCommitted)
		return err
	})
}

func (st *store) rollbackCreated(name string, original os.FileInfo) error {
	current, err := st.files.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect script before rollback: %w", err)
	}
	if !os.SameFile(original, current) {
		return errors.New("script changed before rollback")
	}
	if err := st.files.Remove(name); err != nil {
		return fmt.Errorf("roll back script creation: %w", err)
	}
	return nil
}

// Remove serializes metadata and file removal. A failed metadata commit restores
// the original file without overwriting any newly appeared destination.
func (r *Repository) Remove(name string) error {
	return r.withStore(context.Background(), true, func(st *store) error {
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

func (st *store) remove(script domain.Script, scripts map[string]domain.Script) (err error) {
	// A private staging directory reserves the rollback name without collisions.
	suffix, err := randomName()
	if err != nil {
		return err
	}
	backup := ".remove-" + suffix
	if err := st.files.Mkdir(backup, 0o700); err != nil {
		return fmt.Errorf("reserve rooted removal: %w", err)
	}
	defer func() {
		if cleanupErr := st.files.Remove(backup); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove rollback directory: %w", cleanupErr))
		}
	}()
	tomb := filepath.Join(backup, "script")
	moved := true
	if err := st.files.Rename(script.ScriptFile, tomb); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stage script removal: %w", err)
		}
		moved = false
	}
	delete(scripts, script.Name)
	if err := st.save(scripts); err != nil {
		if moved {
			if errors.Is(err, ErrCommitted) {
				err = errors.Join(err, st.files.Remove(tomb))
			} else {
				err = errors.Join(err, st.restoreRemoved(tomb, script.ScriptFile))
			}
		}
		return err
	}
	if moved {
		if err := st.files.Remove(tomb); err != nil {
			return fmt.Errorf("remove staged script: %w", err)
		}
	}
	return nil
}

func (st *store) restoreRemoved(tomb, name string) error {
	// Link is exclusive, unlike Rename, and cannot clobber a replacement.
	// Retain the staged file if restoring its original name fails.
	if err := st.files.Link(tomb, name); err != nil {
		return fmt.Errorf("restore removed script: %w", err)
	}
	if err := st.files.Remove(tomb); err != nil {
		return fmt.Errorf("remove restored script staging link: %w", err)
	}
	return nil
}

func replacementTarget(st *store, scriptFile string, original *os.File) (os.FileInfo, error) {
	current, err := openRegular(st.files, scriptFile)
	if err != nil {
		return nil, err
	}
	currentInfo, statErr := current.Stat()
	closeErr := current.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		return nil, fmt.Errorf("inspect edit destination: %w", err)
	}
	originalInfo, err := original.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat original script: %w", err)
	}
	if !os.SameFile(currentInfo, originalInfo) {
		return nil, errors.New("script changed while editing")
	}
	return currentInfo, nil
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
	return staging, nil
}

// ReplaceScript atomically replaces the script inode pinned by original after
// rechecking its path. A stale editor is rejected without modifying either file.
func (r *Repository) ReplaceScript(ctx context.Context, scriptFile string, original *os.File, data []byte) error {
	return r.withStore(ctx, true, func(st *store) (err error) {
		if err := st.validate(scriptFile); err != nil {
			return err
		}
		currentInfo, err := replacementTarget(st, scriptFile, original)
		if err != nil {
			return err
		}

		staging, stageErr := r.stageReplacement(st, data, currentInfo.Mode().Perm())
		if staging != "" {
			defer func() {
				if cleanupErr := st.files.RemoveAll(staging); cleanupErr != nil {
					err = errors.Join(err, fmt.Errorf("remove script staging: %w", cleanupErr))
				}
			}()
		}
		if stageErr != nil {
			return stageErr
		}
		stagedName := filepath.Join(staging, "script")

		rename := r.replaceRename
		if rename == nil {
			rename = (*os.Root).Rename
		}
		if err := rename(st.files, stagedName, scriptFile); err != nil {
			return fmt.Errorf("commit edited script: %w", err)
		}
		if err := syncRootDirectory(st.files); err != nil {
			return errors.Join(ErrCommitted, fmt.Errorf("script replacement committed but directory sync failed: %w", err))
		}
		return nil
	})
}

// OpenScript returns a pinned regular file opened beneath script_files without
// following symlinks. The caller must close it and must not reopen its pathname.
func (r *Repository) OpenScript(ctx context.Context, scriptFile string) (*os.File, error) {
	var file *os.File
	err := r.withStore(ctx, false, func(st *store) error {
		if err := st.validate(scriptFile); err != nil {
			return err
		}
		var err error
		file, err = openRegular(st.files, scriptFile)
		return err
	})
	if err != nil && file != nil {
		closeResource(&err, file)
		file = nil
	}
	return file, err
}

// GetScriptsDir returns the canonical script directory for editor display.
func (r *Repository) GetScriptsDir() string { return r.scriptsDir }

type store struct {
	zev                   *os.Root
	files                 *os.Root
	staging               string
	syncMetadataDirectory func(*os.Root) error
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
	st := &store{zev: z, files: files, syncMetadataDirectory: r.syncMetadataDirectory}
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
		if cleanupErr := st.zev.RemoveAll(st.staging); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove metadata staging directory: %w", cleanupErr))
		}
	}()
	return fn(st)
}

func (st *store) load() (map[string]domain.Script, error) {
	scripts := make(map[string]domain.Script)
	if st.zev == nil {
		return scripts, nil
	}
	file, err := openRegular(st.zev, "scripts")
	if errors.Is(err, os.ErrNotExist) {
		return scripts, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open metadata: %w", err)
	}
	data, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var raw map[string]scriptYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse scripts: %w", err)
	}
	for name, meta := range raw {
		if err := st.validate(meta.Script); err != nil {
			return nil, fmt.Errorf("invalid script %q: %w", name, err)
		}
		scripts[name] = domain.Script{Name: name, ScriptFile: meta.Script, Description: meta.Description}
	}
	return scripts, nil
}

func (st *store) save(scripts map[string]domain.Script) error {
	for name, script := range scripts {
		if err := st.validate(script.ScriptFile); err != nil {
			return fmt.Errorf("invalid script %q: %w", name, err)
		}
	}
	if err := checkRegular(st.zev, "scripts"); err != nil {
		return fmt.Errorf("validate metadata: %w", err)
	}
	raw := make(map[string]scriptYAML, len(scripts))
	for name, s := range scripts {
		raw[name] = scriptYAML{Script: s.ScriptFile, Description: s.Description}
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal scripts: %w", err)
	}
	temporary := filepath.Join(st.staging, "scripts")
	file, err := st.zev.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY|noFollowFlags, 0o600)
	if err != nil {
		return fmt.Errorf("create staged metadata: %w", err)
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return fmt.Errorf("write staged metadata: %w", err)
	}
	if err := st.zev.Rename(temporary, "scripts"); err != nil {
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
