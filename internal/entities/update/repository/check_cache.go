package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/entities/update/service"
)

const (
	updateCheckInterval       = 24 * time.Hour
	failedUpdateCheckInterval = time.Hour
	maxUpdateCheckCacheBytes  = 4096
)

// CachedCheck stores the last successful or failed update-check attempt.
type CachedCheck struct {
	CheckedAt time.Time      `json:"checked_at"` //nolint:tagliatelle // Preserve the published cache key.
	Branch    service.Branch `json:"branch"`
	Current   string         `json:"current"`
	Latest    string         `json:"latest,omitempty"`
	Available bool           `json:"available"`
	Failed    bool           `json:"failed,omitempty"`
}

// CheckCache stores update-check state without credentials or user data.
type CheckCache struct{ path string }

// NewCheckCache creates an update-check cache at path.
func NewCheckCache(path string) *CheckCache { return &CheckCache{path: path} }

// IsFresh reports whether entry is usable within its success or failure cache window.
func (c *CheckCache) IsFresh(entry CachedCheck, now time.Time) bool {
	if entry.CheckedAt.IsZero() {
		return false
	}
	age := now.Sub(entry.CheckedAt)
	if age < 0 {
		return false
	}
	interval := updateCheckInterval
	if entry.Failed {
		interval = failedUpdateCheckInterval
	}
	return age < interval
}

// Load reads a valid cached check. A missing or malformed cache is treated as absent.
func (c *CheckCache) Load() (CachedCheck, bool, error) {
	data, err := filestore.ReadFileLimit(c.path, maxUpdateCheckCacheBytes)
	if errors.Is(err, os.ErrNotExist) {
		return CachedCheck{}, false, nil
	}
	if err != nil {
		return CachedCheck{}, false, fmt.Errorf("read update check cache: %w", err)
	}
	var entry CachedCheck
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !decodeCachedCheck(decoder, &entry) {
		return CachedCheck{}, false, nil
	}
	if !hasOnlyJSONEOF(decoder) {
		return CachedCheck{}, false, nil
	}
	if entry.Branch != service.Stable && entry.Branch != service.Main || entry.CheckedAt.IsZero() || entry.Current == "" || entry.Available && entry.Latest == "" {
		return CachedCheck{}, false, nil
	}
	return entry, true, nil
}

func decodeCachedCheck(decoder *json.Decoder, entry *CachedCheck) bool {
	return decoder.Decode(entry) == nil
}

func hasOnlyJSONEOF(decoder *json.Decoder) bool {
	var trailing any
	err := decoder.Decode(&trailing)
	return errors.Is(err, io.EOF)
}

// Refresh checks at most once per 24 hours across cooperating Zev processes.
// The callback runs while the persistent cache lock is held.
func (c *CheckCache) Refresh(ctx context.Context, now time.Time, branch service.Branch, current string, check func(context.Context) (service.CheckResult, error)) (entry CachedCheck, checked bool, err error) {
	if ctx == nil || check == nil || current == "" || branch != service.Stable && branch != service.Main {
		return CachedCheck{}, false, errors.New("invalid update check request")
	}
	if err := filestore.WithLockContext(ctx, c.path, func() error {
		cached, ok, readErr := c.Load()
		if readErr != nil {
			return readErr
		}
		if ok && cached.Branch == branch && c.IsFresh(cached, now) {
			entry = cached
			entry.Current = current
			entry.Available = !entry.Failed && service.UpdateAvailable(branch, current, entry.Latest)
			return nil
		}
		checked = true
		result, checkErr := check(ctx)
		entry = CachedCheck{CheckedAt: now.UTC(), Branch: branch, Current: current, Failed: checkErr != nil}
		if checkErr == nil {
			entry.Latest = result.Version
			entry.Available = result.Update
		}
		data, marshalErr := json.Marshal(entry)
		if marshalErr != nil {
			return fmt.Errorf("encode update check cache: %w", marshalErr)
		}
		if writeErr := filestore.AtomicWrite(c.path, data, 0o600); writeErr != nil {
			return fmt.Errorf("write update check cache: %w", writeErr)
		}
		return nil
	}); err != nil {
		return entry, checked, fmt.Errorf("refresh update check: %w", err)
	}
	return entry, checked, nil
}
