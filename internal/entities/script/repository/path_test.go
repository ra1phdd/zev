package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func testDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

const (
	safeName        = "safe"
	safeFile        = "safe.sh"
	ancestorSymlink = "ancestor symlink"
	metadataSymlink = "metadata symlink"
)

func TestSaveAllRejectsUnsafePathsWithoutEffects(t *testing.T) {
	t.Parallel()
	for _, attack := range []string{metadataSymlink, "invalid script path", "scripts directory symlink", ancestorSymlink} {
		t.Run(attack, func(t *testing.T) {
			dir := testDir(t)
			outside := testDir(t)
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			repo := New(dir)
			file := safeFile
			var link, target string
			switch attack {
			case metadataSymlink:
				link, target = repo.scriptsYAMLPath, sentinel
			case "invalid script path":
				file = "../sentinel"
			case "scripts directory symlink":
				link, target = repo.scriptsDir, outside
			case ancestorSymlink:
				link, target = filepath.Join(outside, "linked"), dir
				repo = New(link)
			}
			if link != "" {
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
			}
			err := repo.SaveAll(map[string]domain.Script{safeName: {Name: safeName, ScriptFile: file}})
			if err == nil {
				t.Errorf("SaveAll accepted %s", attack)
			}
			if data, err := os.ReadFile(filepath.Clean(sentinel)); err != nil || string(data) != "untouched" {
				t.Errorf("external sentinel changed: %q, %v", data, err)
			}
			if attack != metadataSymlink {
				if _, err := os.Lstat(filepath.Join(dir, "scripts")); !os.IsNotExist(err) {
					t.Errorf("metadata created on rejection: %v", err)
				}
			}
		})
	}
}

func TestLoadAllRejectsUnsafeMetadata(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"../outside.sh", "nested/../../outside.sh", "/outside.sh", "", ".", "..", "nested/../safe.sh", `C:\outside.sh`, `nested\outside.sh`} {
		t.Run(file, func(t *testing.T) {
			dir := testDir(t)
			data := fmt.Sprintf("safe:\n  script: %q\n", file)
			if err := os.WriteFile(filepath.Join(dir, "scripts"), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			scripts, err := New(dir).LoadAll()
			if err == nil || scripts != nil {
				t.Errorf("LoadAll returned scripts for unsafe path %q: %v, %v", file, scripts, err)
			}
		})
	}
}

func TestValidateScriptPathRejectsSymlinksAndNonRegularFiles(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"file symlink", "dangling symlink", "internal symlink", "directory file", "scripts symlink", "nested symlink", ancestorSymlink} {
		t.Run(kind, func(t *testing.T) {
			dir := testDir(t)
			outside := testDir(t)
			scriptsDir := filepath.Join(dir, "script_files")
			if err := os.Mkdir(scriptsDir, 0o700); err != nil {
				t.Fatal(err)
			}
			file := safeFile
			target := filepath.Join(outside, safeFile)
			if err := os.WriteFile(target, []byte("external sentinel"), 0o600); err != nil {
				t.Fatal(err)
			}
			var link, destination string
			switch kind {
			case "file symlink":
				link, destination = filepath.Join(scriptsDir, file), target
			case "dangling symlink":
				link, destination = filepath.Join(scriptsDir, file), filepath.Join(outside, "missing")
			case "internal symlink":
				link, destination = filepath.Join(scriptsDir, file), "other.sh"
				if err := os.WriteFile(filepath.Join(scriptsDir, destination), []byte("internal"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory file":
				if err := os.Mkdir(filepath.Join(scriptsDir, file), 0o700); err != nil {
					t.Fatal(err)
				}
			case "scripts symlink":
				if err := os.Remove(scriptsDir); err != nil {
					t.Fatal(err)
				}
				link, destination = scriptsDir, outside
			case "nested symlink":
				file = "nested/safe.sh"
				link, destination = filepath.Join(scriptsDir, "nested"), outside
			case ancestorSymlink:
				link, destination = filepath.Join(testDir(t), "linked"), dir
				dir = link
			}
			if link != "" {
				if err := os.Symlink(destination, link); err != nil {
					t.Fatal(err)
				}
			}
			if path, err := New(dir).ValidateScriptPath(file); err == nil {
				t.Errorf("accepted %s: %s", kind, path)
			}
		})
	}
}
