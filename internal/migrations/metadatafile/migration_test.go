package metadatafile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/migrations/aliases"
	"github.com/ra1phdd/zev/internal/migrations/filetxn"
	"github.com/ra1phdd/zev/internal/migrations/scripts"
)

const (
	aliasStore  = "aliases"
	scriptStore = "scripts"
)

func write(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Clean(path), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExplicitMigration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		entity, source string
		migrate        func(context.Context, string) error
	}{
		{aliasStore, "plain: echo\nshow: echo {{.arg1}}\n", aliases.Migrate},
		{scriptStore, "show: {script: show.sh, description: my description}\n", scripts.Migrate},
	} {
		t.Run(tc.entity, func(t *testing.T) {
			dir := t.TempDir()
			active := filepath.Join(dir, tc.entity)
			source := tc.source
			write(t, active, source)
			files := filepath.Join(dir, "script_files")
			if err := os.Mkdir(files, 0o700); err != nil {
				t.Fatal(err)
			}
			body := filepath.Join(files, "show.sh")
			write(t, body, "echo hello")
			lock := active
			if tc.entity == scriptStore {
				lock = filepath.Join(filepath.Dir(dir), "."+filepath.Base(dir)+"-scripts")
			}
			err := filestore.WithLock(lock, func() error {
				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				defer cancel()
				if err := tc.migrate(ctx, dir); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("writer lock not honored: %v", err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(dir, "."+tc.entity+"-migration")
			if err := os.Mkdir(journal, 0o700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(journal, "marker")
			backup := filepath.Join(journal, "user.archive")
			write(t, marker, "unresolved")
			write(t, backup, source)
			if err := tc.migrate(t.Context(), dir); err == nil {
				t.Fatal("accepted unresolved journal")
			}
			if string(read(t, active)) != source || string(read(t, marker)) != "unresolved" || string(read(t, backup)) != source {
				t.Fatal("modified pending state")
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			if err := tc.migrate(t.Context(), dir); err != nil {
				t.Fatal(err)
			}
			data := read(t, active)
			if tc.entity == scriptStore {
				m, err := metadata.DecodeScripts(data)
				if err != nil || m["show"].RecordID == "" || m["show"].Description != "my description" || string(read(t, body)) != "echo hello" {
					t.Fatalf("scripts=%v err=%v", m, err)
				}
			} else {
				m, err := metadata.DecodeAliases(data)
				if err != nil || m["plain"].Template != "echo {{.args}}" || m["show"].Template != "echo {{.arg1}}" {
					t.Fatalf("aliases=%v err=%v", m, err)
				}
			}
			if err := tc.migrate(t.Context(), dir); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, read(t, active)) || string(read(t, backup)) != source {
				t.Fatal("modified current data or backup")
			}
			entries, err := os.ReadDir(journal)
			if err != nil || len(entries) != 1 || entries[0].Name() != "user.archive" {
				t.Fatalf("journal directory changed: %v %v", entries, err)
			}
		})
	}
}

func TestMigrationRejectsSameBytesInReplacementFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, aliasStore)
	previous := filepath.Join(dir, "previous")
	const source = "plain: echo\n"
	write(t, active, source)
	err := filestore.WithLockContext(t.Context(), active, func() error {
		return aliases.Schema().Apply(t.Context(), dir, func(_ []byte, publishing bool) error {
			if !publishing {
				t.Fatal("expected legacy migration")
			}
			if err := os.Rename(active, previous); err != nil {
				t.Fatal(err)
			}
			write(t, active, source)
			return nil
		})
	})
	if !errors.Is(err, filetxn.ErrConflict) {
		t.Fatalf("migration result = %v, want source conflict", err)
	}
	if string(read(t, active)) != source || string(read(t, previous)) != source {
		t.Fatal("migration modified the source or replacement")
	}
}
