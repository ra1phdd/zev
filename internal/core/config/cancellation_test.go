package config

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

func TestLoadContextCancelsWhileConfigurationLockIsHeld(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".zev", configFilename)
	err := filestore.WithLock(path, func() error {
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		finished := make(chan error, 1)
		go func() { _, err := LoadContext(ctx); finished <- err }()
		select {
		case err := <-finished:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("load error=%v, want deadline", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("configuration loader ignored cancellation while waiting for its lock")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
