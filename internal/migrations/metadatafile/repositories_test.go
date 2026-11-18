package metadatafile_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
)

func TestRepositoryLeavesCurrentAndUnsupportedMetadataUntouched(t *testing.T) {
	t.Parallel()
	for _, entity := range []string{aliasStore, scriptStore} {
		for _, tc := range []struct {
			version string
			valid   bool
		}{{"2", true}, {"1", false}, {"3", false}, {"'2'", false}, {"null", false}} {
			t.Run(entity+"/"+tc.version, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, entity)
				source := "# retained\nversion: " + tc.version + "\n" + entity + ": {}\n"
				if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
					t.Fatal(err)
				}
				before, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if entity == aliasStore {
					_, err = aliasrepo.New(dir).LoadAllContext(t.Context())
				} else {
					_, err = scriptrepo.New(dir).LoadAllContext(t.Context())
				}
				if (err == nil) != tc.valid {
					t.Fatalf("unexpected load result: %v", err)
				}
				after, err := os.Stat(path)
				if err != nil || !os.SameFile(before, after) {
					t.Fatalf("metadata replaced: %v", err)
				}
				data, err := os.ReadFile(filepath.Clean(path))
				if err != nil || string(data) != source {
					t.Fatalf("metadata changed: %v", err)
				}
				if _, err := os.Stat(filepath.Join(dir, "."+entity+"-migration")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected migration state: %v", err)
				}
			})
		}
	}
}
