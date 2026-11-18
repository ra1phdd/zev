package filetxn

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

const (
	sourceData = "legacy fixture"
	outputData = "converted fixture"
)

func fixtureConverter([]byte) ([]byte, error) { return []byte(outputData), nil }
func fixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func checkContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil || string(got) != want {
		t.Fatalf("unexpected contents in %s: %v", filepath.Base(path), err)
	}
}

func TestMigrationRecovery(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"prepared", "staged", "published", phaseUnlinked} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source := filepath.Join(dir, "config")
			dest := source + ".yaml"
			fixture(t, source, sourceData)
			s, err := snapshotFile(source, false)
			if err != nil {
				t.Fatal(err)
			}
			fixture(t, dest+suffix, string(journalBytes(s, []byte(outputData))))
			switch phase {
			case "staged":
				fixture(t, dest+suffix+".stage", outputData)
			case "published", phaseUnlinked:
				fixture(t, dest, outputData)
			}
			if phase == phaseUnlinked {
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := Migrate(source, dest, fixtureConverter); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(dest) {
				t.Fatalf("unexpected recovery artifacts: %v, %v", entries, err)
			}
			checkContent(t, dest, outputData)
		})
	}
}

func TestMigrationRechecksSourceIdentityAndContent(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"same-content replacement", editSource, newDestination} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source := filepath.Join(dir, "config")
			dest := source + ".yaml"
			fixture(t, source, sourceData)
			convert := func([]byte) ([]byte, error) {
				switch change {
				case "same-content replacement":
					temp := filepath.Join(dir, "replacement")
					fixture(t, temp, sourceData)
					if err := os.Rename(temp, source); err != nil {
						t.Fatal(err)
					}
				case editSource:
					fixture(t, source, "changed")
				case newDestination:
					fixture(t, dest, "user-owned")
				}
				return []byte(outputData), nil
			}
			if err := Migrate(source, dest, convert); err == nil {
				t.Fatal("accepted concurrent replacement")
			}
			want := sourceData
			if change == editSource {
				want = "changed"
			}
			checkContent(t, source, want)
			if change == newDestination {
				checkContent(t, dest, "user-owned")
			}
		})
	}
}

func TestDurabilityFailureRetainsSourceUntilRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "config")
	dest := source + ".yaml"
	fixture(t, source, sourceData)
	failure := errors.New("test sync failure")
	err := migrate(source, dest, fixtureConverter, func(string) error { return failure })
	if !errors.Is(err, failure) || !errors.Is(err, filestore.ErrCommitted) {
		t.Fatalf("unexpected sync failure: %v", err)
	}
	checkContent(t, source, sourceData)
	checkContent(t, dest, outputData)
	if err := Migrate(source, dest, fixtureConverter); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source not removed after durable retry")
	}
}

func TestConflictingPendingPublication(t *testing.T) {
	t.Parallel()
	for _, change := range []string{sourceLeaf, destinationLeaf, "journal", "stage"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source := filepath.Join(dir, "config")
			dest := source + ".yaml"
			fixture(t, source, sourceData)
			src, err := snapshotFile(source, false)
			if err != nil {
				t.Fatal(err)
			}
			fixture(t, dest+suffix, string(journalBytes(src, []byte(outputData))))
			if change == "stage" {
				fixture(t, dest+suffix+".stage", "different")
			} else {
				fixture(t, dest, outputData)
				switch change {
				case sourceLeaf:
					fixture(t, source, "different")
				case destinationLeaf:
					fixture(t, dest, "different")
				case "journal":
					fixture(t, dest+suffix, "different")
				}
			}
			if err := Migrate(source, dest, fixtureConverter); !errors.Is(err, ErrConflict) {
				t.Fatalf("expected conflict, got %v", err)
			}
			if _, err := os.Lstat(source); err != nil {
				t.Fatal("lost legacy source")
			}
		})
	}
}

func TestSafeReadsAndModeRepair(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	fixture(t, path, sourceData)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil { //nolint:gosec // Synthetic historical mode verifies descriptor-based repair.
			t.Fatal(err)
		}
	}
	for _, repair := range []bool{false, true} {
		s, err := snapshotFile(path, repair)
		if err != nil || !bytes.Equal(s.data, []byte(sourceData)) {
			t.Fatalf("read failed: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o644)
		if repair {
			want = 0o600
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != want {
			t.Fatal("unexpected file mode")
		}
	}
	fixture(t, path, string(make([]byte, MaxBytes+1)))
	if _, err := Read(path); err == nil {
		t.Fatal("accepted oversized file")
	}
}

func TestMigrationRefusesUnsafeFiles(t *testing.T) {
	t.Parallel()
	for _, leaf := range []string{sourceLeaf, destinationLeaf, "marker"} {
		for _, kind := range []string{"symlink", "hardlink"} {
			t.Run(leaf+"/"+kind, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				source := filepath.Join(dir, "config")
				dest := source + ".yaml"
				victim := filepath.Join(dir, "victim")
				fixture(t, victim, sourceData)
				target := source
				if leaf != sourceLeaf {
					fixture(t, source, sourceData)
					target = dest
					if leaf == "marker" {
						target = dest + suffix
					}
				}
				link := os.Link
				if kind == "symlink" {
					link = os.Symlink
				}
				if err := link(victim, target); err != nil {
					t.Skipf("cannot create fixture: %v", err)
				}
				if err := Migrate(source, dest, fixtureConverter); err == nil {
					t.Fatal("accepted unsafe migration file")
				}
				if _, err := os.Lstat(source); err != nil {
					t.Fatal("lost source")
				}
				checkContent(t, victim, sourceData)
			})
		}
	}
}

const (
	phaseUnlinked   = "unlinked"
	newDestination  = "destination appears"
	destinationLeaf = "destination"
)

const (
	editSource = "in-place edit"
	sourceLeaf = "source"
)
