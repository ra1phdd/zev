// Package repository safely replaces the running installation and runs its migrations.
package repository

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

const probeMarker = "zev-update-v1\n"

// Installation owns a cooperative update transaction. Parent directories must be
// trusted. Windows replacement is deliberately unsupported rather than renaming
// a running image non-atomically. No privilege escalation or rollback is used.
type Installation struct {
	executable     func() (string, error)
	currentVersion string
	mu             sync.Mutex
	locked         bool
	path           string
	identity       os.FileInfo
}

// New creates an adapter for os.Executable, binding updates to currentVersion.
// An empty currentVersion omits the running-version check for isolated fixtures.
func New(currentVersion string) *Installation {
	return &Installation{executable: os.Executable, currentVersion: currentVersion}
}

// Executable resolves symlinks and validates a regular executable target.
func (r *Installation) Executable() (string, error) {
	path, err := r.executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlinks: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	if _, err := executableInfo(path); err != nil {
		return "", err
	}
	return path, nil
}

func executableInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect executable: %w", err)
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		return nil, errors.New("installation target must be a regular executable")
	}
	return info, nil
}

// WithLock retains the file lock across release selection, installation, and
// migration. A target changed while waiting is refused; callers must restart.
// The callback must not call WithLock recursively or concurrently on this object.
func (r *Installation) WithLock(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update lock: %w", err)
	}
	if runtime.GOOS == "windows" {
		return errors.New("self-update is not supported on Windows; install the verified release manually")
	}
	path, err := r.Executable()
	if err != nil {
		return err
	}
	before, err := executableInfo(path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	err = filestore.WithLockContext(ctx, path+".update", func() error {
		now, err := executableInfo(path)
		if err != nil {
			return err
		}
		if !sameIdentity(before, now) {
			return errors.New("installed executable changed while waiting for update lock; restart zev update")
		}
		if r.currentVersion != "" {
			build, err := probeOutput(ctx, path, "--update-build")
			if err != nil {
				return err
			}
			if build != r.currentVersion+"\n" {
				return errors.New("running version differs from the installed executable; restart zev update")
			}
		}
		r.path = path
		r.identity = now
		r.locked = true
		defer func() { r.locked = false }()
		return fn()
	})
	if err != nil {
		return fmt.Errorf("locked update: %w", err)
	}
	return nil
}

func sameIdentity(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}

// Install stages and probes a native binary before atomically replacing the
// target. It must be called inside WithLock. A committed durability error returns
// the installed path and wraps filestore.ErrCommitted; never roll back it.
func (r *Installation) Install(ctx context.Context, data []byte, version string) (path string, err error) { //nolint:gocognit // The ordered staging, probe, identity, publish, and durability gates are one atomic transaction.
	if !r.locked {
		return "", errors.New("install requires update lock")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("install: %w", err)
	}
	if len(data) == 0 || len(data) > 64<<20 {
		return "", errors.New("invalid update binary size")
	}
	stage, err := os.CreateTemp(filepath.Dir(r.path), ".zev-update-*")
	if err != nil {
		return "", fmt.Errorf("create private executable stage: %w", err)
	}
	stagePath := stage.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, stage.Close())
		}
		if removeErr := os.Remove(stagePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove executable stage: %w", removeErr))
		}
	}()
	if _, err := stage.Write(data); err != nil {
		return "", fmt.Errorf("write executable stage: %w", err)
	}
	if err := stage.Chmod(r.identity.Mode().Perm()); err != nil {
		return "", fmt.Errorf("make stage executable: %w", err)
	}
	if err := stage.Sync(); err != nil {
		return "", fmt.Errorf("sync executable stage: %w", err)
	}
	err = stage.Close()
	closed = true
	if err != nil {
		return "", fmt.Errorf("close executable stage: %w", err)
	}
	if err := validateNative(stagePath); err != nil {
		return "", err
	}
	protocol, err := probeOutput(ctx, stagePath, "--update-protocol")
	if err != nil {
		return "", err
	}
	if protocol != probeMarker {
		return "", errors.New("candidate does not support update protocol zev-update-v1; installed executable retained")
	}
	build, err := probeOutput(ctx, stagePath, "--update-build")
	if err != nil {
		return "", err
	}
	if strings.TrimPrefix(build, "v") != version+"\n" {
		return "", errors.New("candidate executable version does not match the selected release; installed executable retained")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("publish executable: %w", err)
	}
	current, err := executableInfo(r.path)
	if err != nil {
		return "", err
	}
	if !sameIdentity(r.identity, current) {
		return "", errors.New("installation target changed before publication")
	}
	if err := os.Rename(stagePath, r.path); err != nil {
		return "", fmt.Errorf("replace executable (check installation directory permissions; no sudo is attempted): %w", err)
	}
	path = r.path
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return path, errors.Join(filestore.ErrCommitted, fmt.Errorf("open installed directory: %w", err))
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return path, errors.Join(filestore.ErrCommitted, fmt.Errorf("sync installed directory: %w", err))
	}
	return path, nil
}

func validateNative(path string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("candidate is not a Go executable: %w", err)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH {
		return errors.New("candidate executable platform does not match this installation")
	}
	return nil
}

func probeOutput(ctx context.Context, path, flag string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out limitedOutput
	cmd := exec.CommandContext(ctx, path, "version", flag) //nolint:gosec // Only the validated installed or checksum-verified staged executable and fixed internal probe flags reach this call.
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	cmd.Stdin = nil
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("executable update protocol probe failed: %w", err)
	}
	return out.String(), nil
}

type limitedOutput struct {
	buffer bytes.Buffer
}

func (b *limitedOutput) String() string { return b.buffer.String() }
func (b *limitedOutput) Len() int       { return b.buffer.Len() }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 1024 {
		return 0, errors.New("candidate protocol output exceeds limit")
	}
	n, err := b.buffer.Write(p)
	if err != nil {
		return n, fmt.Errorf("capture candidate output: %w", err)
	}
	return n, nil
}

// RunMigrations executes the installed target with literal migration-only argv.
// Output is inherited so progress and failures remain visible. Binary rollback is
// never attempted because data migrations may have partially committed.
func (r *Installation) RunMigrations(ctx context.Context, path string) error {
	if !r.locked || path != r.path {
		return errors.New("migrations require locked installation target")
	}
	cmd := exec.CommandContext(ctx, path, "update", "--migrate-only")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installed executable retained; retry zev update or zev update --migrate-only: %w", err)
	}
	return nil
}
