package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

// Tests change the process environment and global configuration path.
func TestSetupMode(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErr    bool
		wantOut    string
		failOutput bool
	}{
		{
			name:    "valid inject mode",
			args:    []string{executionModeInject},
			wantOut: "✓ Command execution mode set to: inject\n",
		},
		{
			name:    "valid execute mode",
			args:    []string{executionModeExecute},
			wantOut: "✓ Command execution mode set to: execute\n",
		},
		{name: "invalid mode", args: []string{"invalid"}, wantErr: true},
		{name: "show current mode", wantOut: "Current command execution mode: inject\n"},
		{name: "current mode output error", wantErr: true, failOutput: true},
		{name: "confirmation output error", args: []string{executionModeExecute}, wantErr: true, failOutput: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Preserve the caller's environment even though loading dotenv sets it.
			t.Setenv("COMMAND_EXECUTION_MODE", "")
			if err := os.Unsetenv("COMMAND_EXECUTION_MODE"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config")
			originalPath := config.SetConfigPathForTests(path)
			t.Cleanup(func() { config.SetConfigPathForTests(originalPath) })

			if err := config.Save(config.DefaultConfig()); err != nil {
				t.Fatalf("failed to save default config: %v", err)
			}

			var out bytes.Buffer
			outputErr := errors.New("mode output unavailable")
			cmd := newSetupModeCmd()
			cmd.SetArgs(tt.args)
			cmd.SetErr(io.Discard)
			cmd.SetOut(&out)
			if tt.failOutput {
				cmd.SetOut(failingOutput{err: outputErr})
			}
			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.failOutput && !errors.Is(err, outputErr) {
				t.Errorf("Execute() error = %v, want output error", err)
			}
			if !tt.wantErr && out.String() != tt.wantOut {
				t.Errorf("unexpected output: got %q, want %q", out.String(), tt.wantOut)
			}

			if len(tt.args) > 0 && (!tt.wantErr || tt.failOutput) {
				if err := os.Unsetenv("COMMAND_EXECUTION_MODE"); err != nil {
					t.Fatal(err)
				}
				loadedCfg, err := config.Load()
				if err != nil {
					t.Fatalf("failed to load config after save: %v", err)
				}
				if loadedCfg.CommandExecutionMode != tt.args[0] {
					t.Errorf("CommandExecutionMode = %v, want %v", loadedCfg.CommandExecutionMode, tt.args[0])
				}
			}
		})
	}
}
