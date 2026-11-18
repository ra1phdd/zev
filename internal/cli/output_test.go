package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type failingOutput struct {
	err error
}

func (w failingOutput) Write(_ []byte) (int, error) {
	return 0, w.err
}

// Commands resolve storage through the process-wide HOME environment variable.
func TestCommandOutputErrors(t *testing.T) {
	const name = "hello"
	tests := []struct {
		name       string
		newCommand func() *cobra.Command
		args       []string
		aliases    string
		scripts    string
	}{
		{name: "version", newCommand: newVersionCmd},
		{name: "empty aliases", newCommand: newAliasListCmd},
		{name: "alias table", newCommand: newAliasListCmd, aliases: "version: 2\naliases:\n  hello:\n    template: echo hello\n"},
		{name: "add alias", newCommand: newAliasAddCmd, args: []string{name, "echo hello"}},
		{name: "remove alias", newCommand: newAliasRemoveCmd, args: []string{name}, aliases: "version: 2\naliases:\n  hello:\n    template: echo hello\n"},
		{name: "empty scripts", newCommand: newScriptListCmd},
		{name: "script table", newCommand: newScriptListCmd, scripts: "version: 2\nscripts:\n  hello:\n    recordID: 550e8400-e29b-41d4-a716-446655440000\n    script: hello.sh\n    description: greeting\n"},
		{name: "create script", newCommand: newScriptCreateCmd, args: []string{name, "greeting", managedBodyStdinFlag}},
		{name: "remove script", newCommand: newScriptRemoveCmd, args: []string{name}, scripts: "version: 2\nscripts:\n  hello:\n    recordID: 550e8400-e29b-41d4-a716-446655440000\n    script: hello.sh\n"},
		{name: "empty history", newCommand: newHistoryCmd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SKIP_EDITOR", "1")
			dir := filepath.Join(home, ".zev")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if tt.aliases != "" {
				if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte(tt.aliases), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tt.scripts != "" {
				if err := os.WriteFile(filepath.Join(dir, "scripts"), []byte(tt.scripts), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			outputErr := errors.New("output unavailable")
			cmd := tt.newCommand()
			cmd.SetIn(strings.NewReader("#!/bin/bash\n"))
			cmd.SetOut(failingOutput{err: outputErr})
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); !errors.Is(err, outputErr) {
				t.Fatalf("Execute() error = %v, want output error", err)
			}
		})
	}
}
