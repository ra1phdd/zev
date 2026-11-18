package scripts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateChecksBodiesBeforePublication(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"missing", "traversal", "symlink", "symlink directory", "hardlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			files := filepath.Join(dir, "script_files")
			if err := os.Mkdir(files, 0o700); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(dir, "other")
			if err := os.WriteFile(other, []byte("body"), 0o600); err != nil {
				t.Fatal(err)
			}
			name := "show.sh"
			body := filepath.Join(files, name)
			var err error
			switch kind {
			case "directory":
				err = os.Mkdir(body, 0o700)
			case "symlink":
				err = os.Symlink(other, body)
			case "hardlink":
				err = os.Link(other, body)
			case "symlink directory":
				err = os.Symlink(dir, body)
				name += "/other"
			case "traversal":
				name = "../other"
			}
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "scripts")
			source := "show: {script: " + name + ", description: preserved}\n"
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(t.Context(), dir); err == nil {
				t.Fatal("accepted unsafe or absent body")
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil || string(got) != source {
				t.Fatalf("source changed: %v", err)
			}
		})
	}
}
