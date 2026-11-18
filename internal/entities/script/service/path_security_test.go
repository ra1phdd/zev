package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
)

const (
	safeScriptFile   = "safe.sh"
	newScriptName    = "new"
	executeOperation = "execute"
	createOperation  = "create"
	removeOperation  = "remove"
	ancestorSymlink  = "ancestor symlink"
	metadataSymlink  = "metadata symlink"
)

func TestServiceRejectsUnsafeScriptFiles(t *testing.T) {
	t.Parallel()
	for _, attack := range []string{"traversal", "absolute", "file symlink", "directory file", "scripts symlink", "nested symlink", ancestorSymlink, metadataSymlink} {
		for _, operation := range []string{createOperation, executeOperation, removeOperation, "list"} {
			t.Run(attack+"/"+operation, func(t *testing.T) {
				svc, dir := newTestService(t)
				outside, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				sentinel := filepath.Join(outside, safeScriptFile)
				original := "#!/usr/bin/env bash\nprintf changed > \"$1\"\n"
				if err := os.WriteFile(sentinel, []byte(original), 0o600); err != nil {
					t.Fatal(err)
				}
				scriptsDir := filepath.Join(dir, "script_files")
				if err := os.Mkdir(scriptsDir, 0o700); err != nil {
					t.Fatal(err)
				}
				file := safeScriptFile
				var link, target string
				switch attack {
				case "traversal":
					file, err = filepath.Rel(scriptsDir, sentinel)
					if err != nil {
						t.Fatal(err)
					}
				case "absolute":
					file = sentinel
				case "file symlink":
					link, target = filepath.Join(scriptsDir, file), sentinel
				case "directory file":
					if err := os.Mkdir(filepath.Join(scriptsDir, file), 0o700); err != nil {
						t.Fatal(err)
					}
				case "scripts symlink":
					if err := os.Remove(scriptsDir); err != nil {
						t.Fatal(err)
					}
					link, target = scriptsDir, outside
				case "nested symlink":
					file = "nested/safe.sh"
					link, target = filepath.Join(scriptsDir, "nested"), outside
				case ancestorSymlink:
					link, target = filepath.Join(outside, "linked"), dir
				case metadataSymlink:
					link, target = filepath.Join(dir, "scripts"), filepath.Join(outside, "metadata")
				}
				metadata := []byte("version: 2\nscripts:\n  safe:\n    recordID: 550e8400-e29b-41d4-a716-446655440000\n    script: " + file + "\n")
				metadataPath := filepath.Join(dir, "scripts")
				if attack == metadataSymlink {
					metadataPath = target
				}
				if err := os.WriteFile(metadataPath, metadata, 0o600); err != nil {
					t.Fatal(err)
				}
				if link != "" {
					if err := os.Symlink(target, link); err != nil {
						t.Fatal(err)
					}
				}
				if attack == ancestorSymlink {
					svc = New(repository.New(link))
				}
				switch operation {
				case createOperation:
					err = svc.Create(t.Context(), domain.Script{Name: newScriptName, ScriptFile: "new.sh", Description: "unsafe"}, []byte("exit 0\n"))
				case executeOperation:
					err = svc.Execute(t.Context(), safeScriptName, []string{sentinel})
				case removeOperation:
					err = svc.Remove(safeScriptName)
				case "list":
					_, err = svc.List()
				}
				if err == nil {
					t.Errorf("%s accepted %s", operation, attack)
				}
				if data, err := os.ReadFile(filepath.Clean(sentinel)); err != nil || string(data) != original {
					t.Errorf("external sentinel changed: %q, %v", data, err)
				}
				if data, err := os.ReadFile(filepath.Clean(metadataPath)); err != nil || string(data) != string(metadata) {
					t.Errorf("metadata changed on rejection: %q, %v", data, err)
				}
				if _, err := os.Lstat(filepath.Join(scriptsDir, "new.sh")); !os.IsNotExist(err) {
					t.Errorf("script created on rejection: %v", err)
				}
			})
		}
	}
}

func TestCreateDoesNotOverwriteUnregisteredFile(t *testing.T) {
	t.Parallel()
	svc, dir := newTestService(t)
	if err := os.Mkdir(filepath.Join(dir, "script_files"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "script_files", safeScriptFile)
	if err := os.WriteFile(path, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(t.Context(), domain.Script{Name: safeScriptName, ScriptFile: safeScriptFile, Description: "replacement"}, []byte("exit 0\n")); err == nil {
		t.Error("Create overwrote an existing file")
	}
	if data, err := os.ReadFile(filepath.Clean(path)); err != nil || string(data) != "sentinel" {
		t.Errorf("existing file changed: %q, %v", data, err)
	}
}

func TestCreateRejectsUnsafeNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"../sentinel", "nested/../../sentinel", "/absolute", "", ".", "..", "nested/child", `nested\child`, "query", "name with spaces", "colon:name"} {
		t.Run(name, func(t *testing.T) {
			svc, dir := newTestService(t)
			sentinel := filepath.Join(dir, "sentinel.sh")
			if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := svc.Create(t.Context(), domain.Script{Name: name, ScriptFile: safeScriptFile, Description: "unsafe"}, []byte("exit 0\n")); err == nil {
				t.Errorf("Create(%q) succeeded", name)
			}
			data, err := os.ReadFile(filepath.Clean(sentinel))
			if err != nil || string(data) != "untouched" {
				t.Errorf("external sentinel changed: %q, %v", data, err)
			}
			if _, err := os.Lstat(filepath.Join(dir, "scripts")); !os.IsNotExist(err) {
				t.Errorf("metadata created on rejection: %v", err)
			}
		})
	}
}
