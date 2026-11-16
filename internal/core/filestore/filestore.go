// Package filestore provides atomic file replacement and cooperative transactions.
package filestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrCommitted marks a successful replacement whose directory sync failed. The new
// contents are already visible, but crash durability is uncertain. Use errors.Is
// to classify it; callers must not roll back dependent state or blindly retry a
// non-idempotent transaction as though the replacement never happened.
var ErrCommitted = errors.New("storage replacement committed")

// AtomicWrite replaces path with data using a temporary file in the same directory.
// It creates parent directories with mode 0700 and applies mode to the new file.
// Parent directories must be trusted; the destination must be absent or regular.
// The temporary file is synced and closed before rename; failures before rename
// leave an existing destination unchanged. A directory-sync failure wraps both
// ErrCommitted and its cause: errors.Is recognizes each after the new file becomes
// visible. It does not lock: read-modify-write callers must wrap their entire
// transaction in WithLock.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	w := atomicWriter{
		write:    (*os.File).Write,
		syncFile: (*os.File).Sync,
		rename:   os.Rename,
		syncDir:  syncDirectory,
	}
	return w.writeFile(path, data, mode)
}

type atomicWriter struct {
	write    func(*os.File, []byte) (int, error)
	syncFile func(*os.File) error
	rename   func(string, string) error
	syncDir  func(string) error
}

func (w atomicWriter) writeFile(path string, data []byte, mode os.FileMode) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { //nolint:gosec // This storage primitive intentionally accepts caller-selected paths.
		return fmt.Errorf("create storage directory: %w", err)
	}
	if err := validateDestination(path); err != nil {
		return err
	}
	if w.syncFile == nil {
		w.syncFile = (*os.File).Sync
	}
	if w.syncDir == nil {
		w.syncDir = syncDirectory
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := file.Name()
	committed := false
	defer func() {
		err = errors.Join(err, removeTemporaryFile(temporaryPath, committed))
	}()
	closed := false
	defer func() {
		err = errors.Join(err, closeTemporaryFile(file, closed))
	}()
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	n, err := w.write(file, data)
	if err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("write temporary file: %w", io.ErrShortWrite)
	}
	if err := w.syncFile(file); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	err = file.Close()
	closed = true
	if err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := w.rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace storage file: %w", err)
	}
	committed = true
	if err := w.syncDir(filepath.Dir(path)); err != nil {
		return errors.Join(ErrCommitted, fmt.Errorf("directory sync failed: %w", err))
	}
	return nil
}

func removeTemporaryFile(path string, committed bool) error {
	if committed {
		return nil
	}
	if err := os.Remove(path); err != nil { //nolint:gosec // The path was returned by os.CreateTemp.
		return fmt.Errorf("remove temporary file: %w", err)
	}
	return nil
}

func closeTemporaryFile(file *os.File, closed bool) error {
	if closed {
		return nil
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	return nil
}

// ReadFile reads a regular file without following a final symlink or reparse point.
// Parent directories must be trusted; aliases in those directories are allowed.
// Missing files preserve os.IsNotExist compatibility. The opened file is checked
// before reading, so replacing the leaf after validation cannot redirect a read.
func ReadFile(path string) (data []byte, err error) {
	if err := validateDestination(path); err != nil {
		return nil, err
	}
	file, err := openReadFile(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close storage file: %w", closeErr))
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat open storage file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("read storage file: source is not a regular file")
	}
	data, err = io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read storage file: %w", err)
	}
	return data, nil
}

// ValidateDirectory rejects an existing symlink or non-directory at dir, allowing
// an absent directory so WithLock or AtomicWrite can create it later. Call it
// before those operations when dir is an application-owned storage directory.
// Ancestors must be trusted and may contain symlink aliases (for example HOME).
// This check does not fence concurrent hostile changes to the directory tree.
func ValidateDirectory(dir string) error {
	info, err := os.Lstat(filepath.Clean(dir)) //nolint:gosec // Caller selects the application directory; ancestors are trusted.
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat storage directory: %w", err)
	}
	if info.Mode().Type() != os.ModeDir {
		return errors.New("storage directory must be a directory, not a symlink or special file")
	}
	return nil
}

func validateDestination(path string) error {
	info, err := os.Lstat(path) //nolint:gosec // Caller selects the storage path; parents must be trusted.
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat storage file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("storage file must be a regular file")
	}
	return nil
}

func validateLockIdentity(file *os.File, path string) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat open sidecar: %w", err)
	}
	named, err := os.Lstat(path) //nolint:gosec // The caller-selected sidecar is checked against its opened identity.
	if err != nil {
		return fmt.Errorf("stat sidecar path: %w", err)
	}
	if !named.Mode().IsRegular() || !os.SameFile(opened, named) {
		return errors.New("lock sidecar path changed during acquisition")
	}
	return nil
}

func waitLockRetry(ctx context.Context) error {
	timer := time.NewTimer(10 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for storage lock: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

type pathLock struct {
	gate chan struct{}
	refs int
}

var pathLocks = struct {
	sync.Mutex
	byPath map[string]*pathLock
}{byPath: make(map[string]*pathLock)}

func retainPathLock(path string) (*pathLock, func()) {
	pathLocks.Lock()
	lock := pathLocks.byPath[path]
	if lock == nil {
		lock = &pathLock{gate: make(chan struct{}, 1)}
		pathLocks.byPath[path] = lock
	}
	lock.refs++
	pathLocks.Unlock()
	return lock, func() {
		pathLocks.Lock()
		defer pathLocks.Unlock()
		lock.refs--
		if lock.refs == 0 {
			delete(pathLocks.byPath, path)
		}
	}
}

// WithLock runs fn under an exclusive cooperative lock without a deadline.
// See WithLockContext for path, callback and sidecar requirements.
func WithLock(path string, fn func() error) error {
	return WithLockContext(context.Background(), path, fn)
}

// WithLockContext runs fn under a per-path gate and OS-backed exclusive lock.
// Waiting for either lock is cancellable; errors.Is recognizes ctx.Err(). Once fn
// starts, cancellation does not interrupt it: the callback owns its I/O context.
// Paths are made absolute and existing parent symlink aliases are resolved. Parent
// directories must be trusted; this advisory protocol does not fence hostile
// directory changes or writers that ignore it. All cooperating readers and writers
// must use the same path. The persistent path.lock must never be removed or replaced:
// its inode is the cross-process identity. Sidecars must be regular, singly linked
// files, not symlinks/reparse points. Parent directories are created as 0700.
// fn must not reacquire the same lock. Its error is preserved; cleanup errors may
// be joined, so callers should classify causes with errors.Is or errors.As.
func WithLockContext(ctx context.Context, path string, fn func() error) (err error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("acquire storage lock: %w", err)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve storage path: %w", err)
	}
	dir := filepath.Dir(absolutePath)
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:gosec // The storage location is explicitly chosen by the caller.
		return fmt.Errorf("create lock directory: %w", err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolve lock directory: %w", err)
	}
	absolutePath = filepath.Join(dir, filepath.Base(absolutePath))
	lock, release := retainPathLock(absolutePath)
	defer release()
	select {
	case lock.gate <- struct{}{}:
		defer func() { <-lock.gate }()
	case <-ctx.Done():
		return fmt.Errorf("acquire storage gate: %w", ctx.Err())
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("acquire storage lock: %w", err)
	}
	file, err := openLockFile(absolutePath + ".lock")
	if err != nil {
		return fmt.Errorf("open storage lock: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close storage lock: %w", closeErr))
		}
	}()
	if err := lockFile(ctx, file); err != nil {
		return fmt.Errorf("acquire storage lock: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("acquire storage lock: %w", err)
	}
	return fn()
}
