package migrations_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Version conversion is composed before opening a store. A current reader must
// not acquire a dependency on the migration that upgrades its own inputs.
func TestCurrentStoresDoNotImportMigrations(t *testing.T) {
	t.Parallel()
	const migrations = "github.com/ra1phdd/zev/internal/migrations"
	internal := os.DirFS("..")
	for _, dir := range []string{"core", "entities"} {
		err := fs.WalkDir(internal, dir, func(name string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return err
			}
			data, err := fs.ReadFile(internal, name)
			if err != nil {
				return fmt.Errorf("read %s: %w", name, err)
			}
			f, err := parser.ParseFile(token.NewFileSet(), name, data, parser.ImportsOnly)
			if err != nil {
				return fmt.Errorf("parse imports of %s: %w", name, err)
			}
			for _, imp := range f.Imports {
				pkg, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return fmt.Errorf("decode import in %s: %w", name, err)
				}
				if pkg == migrations || strings.HasPrefix(pkg, migrations+"/") {
					t.Errorf("internal/%s imports %s; migrate before opening the current store", name, pkg)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
