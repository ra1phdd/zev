package repository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

func TestLoadAllContextCancelsWhileAliasLockIsHeld(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := filestore.WithLock(filepath.Join(dir, "aliases"), func() error {
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		finished := make(chan error, 1)
		go func() { _, err := New(dir).LoadAllContext(ctx); finished <- err }()
		select {
		case err := <-finished:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("load error=%v, want deadline", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("alias loader ignored cancellation while waiting for its lock")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
