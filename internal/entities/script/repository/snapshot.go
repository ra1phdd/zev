package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// BodySnapshot is an immutable prerequisite binding a registered record and path
// to an opened file's identity and content hash. Its zero value is invalid.
// It does not retain a descriptor; reads and publication revalidate under the store lock.
type BodySnapshot struct {
	name             string
	recordID         string
	scriptFile       string
	info             os.FileInfo
	hash             string
	metadataCaptured bool
	description      string
	arguments        []domain.Argument
}

// Matches reports whether the captured record identity belongs to script.
// It does not inspect storage; publication must still revalidate under the lock.
func (s BodySnapshot) Matches(script domain.Script) bool {
	return s.info != nil && s.name == script.Name && s.recordID == script.RecordID && s.scriptFile == script.ScriptFile
}

// ErrBodyTooLarge indicates that a snapshot read exceeded its requested byte limit.
var ErrBodyTooLarge = errors.New("script body exceeds read limit")

// CaptureScript returns metadata and its body prerequisite from one locked snapshot.
func (r *Repository) CaptureScript(ctx context.Context, name string) (domain.Script, BodySnapshot, error) {
	var script domain.Script
	var snapshot BodySnapshot
	err := r.withStore(ctx, false, func(st *store) error {
		scripts, err := st.load()
		if err != nil {
			return err
		}
		var ok bool
		script, ok = scripts[name]
		if !ok {
			return fmt.Errorf("script %q not found: %w", name, ErrBodyChanged)
		}
		snapshot, err = st.captureSnapshot(ctx, scripts, script)
		return err
	})
	if err != nil {
		return domain.Script{}, BodySnapshot{}, err
	}
	return script, snapshot, nil
}

func (st *store) captureSnapshot(ctx context.Context, scripts map[string]domain.Script, script domain.Script) (snapshot BodySnapshot, err error) {
	file, err := openRegular(st.files, script.ScriptFile)
	if err != nil {
		return BodySnapshot{}, fmt.Errorf("open snapshot source: %w", err)
	}
	defer closeResource(&err, file)
	info, err := file.Stat()
	if err != nil {
		return BodySnapshot{}, fmt.Errorf("inspect snapshot source: %w", err)
	}
	hash, err := hashReader(ctx, file)
	if err != nil {
		return BodySnapshot{}, err
	}
	after, err := file.Stat()
	if err != nil {
		return BodySnapshot{}, fmt.Errorf("inspect captured source: %w", err)
	}
	if !sameBodyInfo(info, after) {
		return BodySnapshot{}, ErrBodyChanged
	}
	snapshot = BodySnapshot{name: script.Name, recordID: script.RecordID, scriptFile: script.ScriptFile, info: info, hash: hash, metadataCaptured: true, description: script.Description, arguments: append([]domain.Argument(nil), script.Arguments...)}
	if err := st.validateSnapshot(ctx, scripts, snapshot); err != nil {
		return BodySnapshot{}, err
	}
	return snapshot, nil
}

func (s BodySnapshot) matchesGenerationMetadata(script domain.Script) bool {
	return !s.metadataCaptured || s.description == script.Description && slices.Equal(s.arguments, script.Arguments)
}

func sameBodyInfo(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}

func hashReader(ctx context.Context, reader io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, snapshotReader{ctx: ctx, reader: reader}); err != nil {
		return "", fmt.Errorf("hash snapshot source: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (st *store) snapshotFile(scripts map[string]domain.Script, snapshot BodySnapshot) (*os.File, error) {
	current, ok := scripts[snapshot.name]
	if !ok || !snapshot.Matches(current) {
		return nil, ErrBodyChanged
	}
	file, err := openRegular(st.files, snapshot.scriptFile)
	if err != nil {
		return nil, errors.Join(ErrBodyChanged, fmt.Errorf("open snapshot destination: %w", err))
	}
	info, err := file.Stat()
	if err != nil || !sameBodyInfo(snapshot.info, info) {
		return nil, errors.Join(ErrBodyChanged, err, file.Close())
	}
	return file, nil
}

func (st *store) validateSnapshot(ctx context.Context, scripts map[string]domain.Script, snapshot BodySnapshot) (err error) {
	file, err := st.snapshotFile(scripts, snapshot)
	if err != nil {
		return err
	}
	defer closeResource(&err, file)
	hash, err := hashReader(ctx, file)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || !sameBodyInfo(snapshot.info, info) || hash != snapshot.hash {
		return errors.Join(ErrBodyChanged, err)
	}
	return nil
}

// ReadBodyParams selects the prerequisite and optional positive byte limit.
// A zero MaxBytes permits an unbounded read for the editor working copy.
type ReadBodyParams struct {
	Snapshot BodySnapshot
	MaxBytes int64
}

// ReadBody returns detached bytes represented by Snapshot, never a reopened path
// without revalidation. Stale sources return ErrBodyChanged; oversized reads return
// ErrBodyTooLarge without returning partial data.
func (r *Repository) ReadBody(ctx context.Context, params ReadBodyParams) ([]byte, error) {
	var body []byte
	err := r.withStore(ctx, false, func(st *store) (err error) {
		scripts, err := st.load()
		if err != nil {
			return err
		}
		file, err := st.snapshotFile(scripts, params.Snapshot)
		if err != nil {
			return err
		}
		defer closeResource(&err, file)
		var reader io.Reader = snapshotReader{ctx: ctx, reader: file}
		if params.MaxBytes > 0 {
			reader = io.LimitReader(reader, params.MaxBytes+1)
		}
		body, err = io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("read snapshot source: %w", err)
		}
		if params.MaxBytes > 0 && int64(len(body)) > params.MaxBytes {
			return ErrBodyTooLarge
		}
		info, err := file.Stat()
		if err != nil || !sameBodyInfo(params.Snapshot.info, info) || hashBytes(body) != params.Snapshot.hash || !params.Snapshot.matchesGenerationMetadata(scripts[params.Snapshot.name]) {
			return errors.Join(ErrBodyChanged, err)
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}

type snapshotReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r snapshotReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, fmt.Errorf("read script snapshot: %w", err)
	}
	n, err := r.reader.Read(p)
	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}
	if err != nil {
		return n, fmt.Errorf("read script snapshot: %w", err)
	}
	return n, nil
}
