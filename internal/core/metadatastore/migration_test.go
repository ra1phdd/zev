package metadatastore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

func TestMigrateArchivesExactSourceAndRecoversPublishedMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "aliases")
	journal := filepath.Join(dir, ".aliases-migration")
	source := []byte("legacy: value\n")
	canonical := []byte("version: 2\naliases: {}\n")
	if err := os.WriteFile(active, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(active, journal, source, canonical); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(active) //nolint:gosec // The test reads its isolated TempDir fixture.
	if err != nil || string(got) != string(canonical) {
		t.Fatalf("active=%q err=%v", got, err)
	}
	entries, err := os.ReadDir(journal)
	if err != nil {
		t.Fatal(err)
	}
	var archived bool
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".archive") {
			data, readErr := os.ReadFile(filepath.Join(journal, entry.Name())) //nolint:gosec // The entry comes from the isolated TempDir journal.
			if readErr != nil {
				t.Fatal(readErr)
			}
			archived = string(data) == string(source)
		}
	}
	if !archived {
		t.Fatal("exact source archive not found")
	}
	if err := Recover(active, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(journal, "marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed migration marker was not removed: %v", err)
	}
}

func TestPublishMigrationArchiveMarkerFailureIsCommitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "aliases")
	journal := filepath.Join(dir, ".aliases-migration")
	source := []byte("legacy: value\n")
	canonical := []byte("version: 2\naliases: {}\n")
	if err := os.WriteFile(active, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureJournal(journal); err != nil {
		t.Fatal(err)
	}
	m := marker{OperationID: "op", SourceHash: hashBytes(source), ArchiveHash: hashBytes(source), ActiveHash: hashBytes(canonical), Archive: "archive", ArchiveTmp: "archive.tmp", ActiveTmp: "active.tmp", Phase: migrationPhasePreparing}
	if err := os.WriteFile(filepath.Join(journal, m.ArchiveTmp), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, m.ActiveTmp), canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	markerFailure := errors.New("archive marker write failed")
	err := publishMigrationWithHooks(active, journal, m, filepath.Join(journal, m.Archive), filepath.Join(journal, m.ActiveTmp), source, func(string, marker) error {
		return markerFailure
	})
	if !errors.Is(err, filestore.ErrCommitted) || !errors.Is(err, markerFailure) {
		t.Fatalf("archive marker failure = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(journal, m.Archive)); statErr != nil {
		t.Fatalf("published archive missing: %v", statErr)
	}
}

func TestRecoverArchivedPhasePublishesStagedCanonical(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "scripts")
	journal := filepath.Join(dir, ".scripts-migration")
	source := []byte("legacy: value\n")
	canonical := []byte("version: 2\nscripts: {}\n")
	if err := os.WriteFile(active, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureJournal(journal); err != nil {
		t.Fatal(err)
	}
	m := marker{OperationID: "op", SourceHash: hashBytes(source), ArchiveHash: hashBytes(source), ActiveHash: hashBytes(canonical), Archive: migrationArchive, ActiveTmp: migrationActiveTmp, Phase: "archived"}
	if err := os.WriteFile(filepath.Join(journal, m.Archive), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, m.ActiveTmp), canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(journal, m); err != nil {
		t.Fatal(err)
	}
	if err := Recover(active, journal); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(active) //nolint:gosec // The test reads its isolated TempDir fixture.
	if err != nil || string(got) != string(canonical) {
		t.Fatalf("active=%q err=%v", got, err)
	}
}

func TestRecoverClearsPublishedMarkerAfterCanonicalMutation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "aliases")
	journal := filepath.Join(dir, ".aliases-migration")
	source := []byte("legacy: value\n")
	canonical := []byte("version: 2\naliases: {}\n")
	if err := os.WriteFile(active, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureJournal(journal); err != nil {
		t.Fatal(err)
	}
	m := marker{OperationID: "op", SourceHash: hashBytes(source), ArchiveHash: hashBytes(source), ActiveHash: hashBytes([]byte("older canonical\n")), Archive: migrationArchive, ActiveTmp: migrationActiveTmp, Phase: migrationPhasePublished}
	if err := os.WriteFile(filepath.Join(journal, m.Archive), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(journal, m); err != nil {
		t.Fatal(err)
	}
	if err := Recover(active, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(journal, "marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published marker was not cleared: %v", err)
	}
}

func TestRecoverMatchingActiveSyncFailureRetainsMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "aliases")
	journal := filepath.Join(dir, ".aliases-migration")
	source := []byte("legacy: value\n")
	canonical := []byte("version: 2\naliases: {}\n")
	if err := os.WriteFile(active, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureJournal(journal); err != nil {
		t.Fatal(err)
	}
	m := marker{OperationID: "op", SourceHash: hashBytes(source), ArchiveHash: hashBytes(source), ActiveHash: hashBytes(canonical), Archive: migrationArchive, ActiveTmp: migrationActiveTmp, Phase: migrationPhaseArchived}
	if err := os.WriteFile(filepath.Join(journal, m.Archive), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(journal, m); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("active directory sync failed")
	var synced string
	err := recoverMarkerWithHooks(active, journal, m, func(path string) error {
		synced = path
		return failure
	}, writeMarker)
	if !errors.Is(err, filestore.ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("matching active recovery = %v", err)
	}
	if synced != dir {
		t.Fatalf("synced directory = %q, want %q", synced, dir)
	}
	got, readErr := readMarker(journal)
	if readErr != nil || got.Phase != migrationPhaseArchived {
		t.Fatalf("marker after failed sync = %#v, %v", got, readErr)
	}
}

func TestPublishRecoveredMetadataMarkerFailureIsCommitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "aliases")
	journal := filepath.Join(dir, ".aliases-migration")
	source := []byte("legacy: value\n")
	canonical := []byte("version: 2\naliases: {}\n")
	if err := os.WriteFile(active, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureJournal(journal); err != nil {
		t.Fatal(err)
	}
	m := marker{OperationID: "op", SourceHash: hashBytes(source), ArchiveHash: hashBytes(source), ActiveHash: hashBytes(canonical), Archive: migrationArchive, ActiveTmp: migrationActiveTmp, Phase: migrationPhaseArchived}
	if err := os.WriteFile(filepath.Join(journal, m.Archive), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, m.ActiveTmp), canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	markerFailure := errors.New("publish marker failed")
	err := publishRecoveredMetadataWithHooks(active, journal, filepath.Join(journal, m.ActiveTmp), m, func(string) error { return nil }, func(string, marker) error {
		return markerFailure
	})
	if !errors.Is(err, filestore.ErrCommitted) || !errors.Is(err, markerFailure) {
		t.Fatalf("marker failure = %v", err)
	}
	got, readErr := os.ReadFile(active) //nolint:gosec // The test reads its isolated TempDir fixture.
	if readErr != nil || string(got) != string(canonical) {
		t.Fatalf("published metadata = %q, %v", got, readErr)
	}
}

func TestMigrationCleanupSyncFailureIsCommitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	journal := filepath.Join(dir, ".aliases-migration")
	if err := ensureJournal(journal); err != nil {
		t.Fatal(err)
	}
	m := marker{OperationID: "op", SourceHash: "source", ArchiveHash: "archive", ActiveHash: "active", Archive: migrationArchive, ActiveTmp: migrationActiveTmp, Phase: migrationPhasePublished}
	if err := writeMarker(journal, m); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("journal directory sync failed")
	err := removeCompletedMarkerWithSync(journal, func(string) error { return failure })
	if !errors.Is(err, filestore.ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("completed marker cleanup = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(journal, "marker")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("marker retained after unlink: %v", statErr)
	}
}

func TestPublishArchiveCleanupFailureIsCommitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tmp := filepath.Join(dir, "archive.tmp")
	final := filepath.Join(dir, "archive")
	data := []byte("archive")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("archive temporary cleanup failed")
	err := publishArchiveWithHooks(tmp, final, hashBytes(data), func(string) error { return nil }, func(string) error { return failure })
	if !errors.Is(err, filestore.ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("archive cleanup = %v", err)
	}
	if got, readErr := os.ReadFile(final); readErr != nil || string(got) != string(data) { //nolint:gosec // The path is inside the isolated test directory.
		t.Fatalf("published archive = %q, %v", got, readErr)
	}
}

func TestRemoveStagingSyncFailureIsCommitted(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), ".migration")
	m := marker{ArchiveTmp: "archive.tmp", ActiveTmp: migrationActiveTmp}
	if err := ensureJournal(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{m.ArchiveTmp, m.ActiveTmp, "marker"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("staged"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("staging journal sync failed")
	err := removeStagingWithSync(dir, m, func(string) error { return failure })
	if !errors.Is(err, filestore.ErrCommitted) || !errors.Is(err, failure) {
		t.Fatalf("staging cleanup = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "marker")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("staging marker retained: %v", statErr)
	}
}

func TestArchivePairIsAcceptedOnlyForPublishedInode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tmp := filepath.Join(dir, "archive.tmp")
	final := filepath.Join(dir, "archive")
	data := []byte("archive")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(tmp, final); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchiveArtifact(tmp, final, hashBytes(data)); err != nil {
		t.Fatalf("owned archive pair rejected: %v", err)
	}
}

func TestMigrationReadersRejectHardlinkedLeaves(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "marker")
	link := filepath.Join(dir, "marker-link")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readMigrationFile(path); err == nil {
		t.Fatal("hard-linked migration leaf accepted")
	}
}

func TestReadMigrationFileRejectsOversizedPayload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "marker")
	if err := os.WriteFile(path, make([]byte, maxMigrationPayload+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMigrationFile(path); err == nil {
		t.Fatal("oversized migration file accepted")
	}
}

func TestRecoverRejectsCorruptMarkerAndCommittedArchiveSync(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	active := filepath.Join(dir, "aliases")
	journal := filepath.Join(dir, ".aliases-migration")
	if err := os.WriteFile(active, []byte("legacy: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, "marker"), []byte(`{"phase":"published"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(active, journal); err == nil {
		t.Fatal("corrupt marker accepted")
	}
}
