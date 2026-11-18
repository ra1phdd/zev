package migrations_test

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

// Version-step filenames are a contributor contract: a search in migrations/
// must find every step, even when the calling repository lives elsewhere.
func TestVersionStepsBelongToMigrationPackages(t *testing.T) {
	t.Parallel()
	internal := os.DirFS("..")
	step := regexp.MustCompile(`^(.+)\.v[0-9]+\.go$`)
	err := fs.WalkDir(internal, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		match := step.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil
		}
		want := "migrations/" + match[1]
		if path.Dir(name) != want {
			t.Errorf("internal/%s: version steps for %s belong in internal/%s", name, match[1], want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.v1.go", "aliases.v2.go", "scripts.v2.go"} {
		format, _, _ := strings.Cut(name, ".")
		if _, err := fs.Stat(internal, path.Join("migrations", format, name)); err != nil {
			t.Error(err)
		}
	}
}
