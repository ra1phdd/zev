package editor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveEditorPrecedence(t *testing.T) {
	const (
		scriptEditor = "custom-script"
		userEditor   = "custom-editor"
		nanoEditor   = "nano"
		vimEditor    = "vim"
		viEditor     = "vi"
	)
	for _, tc := range []struct {
		name         string
		scriptEditor string
		editor       string
		available    []string
		want         string
	}{
		{name: "script editor wins", scriptEditor: scriptEditor, editor: userEditor, available: []string{scriptEditor, userEditor, nanoEditor}, want: scriptEditor},
		{name: "editor fallback", scriptEditor: "missing", editor: userEditor, available: []string{userEditor, nanoEditor}, want: userEditor},
		{name: "nano first", available: []string{nanoEditor, vimEditor, viEditor}, want: nanoEditor},
		{name: "vim fallback", available: []string{vimEditor, viEditor}, want: vimEditor},
		{name: "vi fallback", available: []string{viEditor}, want: viEditor},
		{name: "unavailable", scriptEditor: "missing", editor: "also-missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			t.Setenv("SCRIPT_EDITOR", tc.scriptEditor)
			t.Setenv("EDITOR", tc.editor)
			suffix := ""
			if runtime.GOOS == "windows" {
				t.Setenv("PATHEXT", ".EXE")
				suffix = ".exe"
			}
			for _, name := range tc.available {
				// Resolve only looks up these fixtures; it never executes them.
				if err := os.WriteFile(filepath.Join(dir, name+suffix), nil, 0o700); err != nil { //nolint:gosec // Isolated executable lookup fixture.
					t.Fatal(err)
				}
			}
			if got := Resolve(t.Context()); got != tc.want {
				t.Errorf("Resolve() = %q, want %q", got, tc.want)
			}
		})
	}
}
