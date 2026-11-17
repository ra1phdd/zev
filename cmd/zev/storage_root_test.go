package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/spf13/cobra"
)

// Storage handlers must reject the process home before initializing repositories.
func TestStorageHandlersRejectUntrustedHome(t *testing.T) {
	for _, home := range []string{"", homeStorageRelativePath} {
		t.Run("home="+home, func(t *testing.T) {
			project := t.TempDir()
			t.Chdir(project)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("home", home)
			for _, tc := range []struct {
				name string
				run  func(*cobra.Command, []string) error
				args []string
			}{
				{name: "alias add", run: runAliasAdd, args: []string{machineFixtureName, "echo fixture"}},
				{name: "alias edit", run: runAliasEdit, args: []string{machineFixtureName}},
				{name: "alias last", run: runAliasLast, args: []string{machineFixtureName}},
				{name: "alias list", run: runAliasList},
				{name: "alias remove", run: runAliasRemove, args: []string{machineFixtureName}},
				{name: "script create", run: runScriptCreate, args: []string{machineFixtureName}},
				{name: "script edit", run: runScriptEdit, args: []string{machineFixtureName}},
				{name: "script show", run: runScriptShow, args: []string{machineFixtureName}},
				{name: "script list", run: runScriptList},
				{name: "script remove", run: runScriptRemove, args: []string{machineFixtureName}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cmd := &cobra.Command{}
					cmd.SetContext(t.Context())
					cmd.SetOut(io.Discard)
					cmd.SetErr(io.Discard)
					if err := tc.run(cmd, tc.args); !errors.Is(err, config.ErrInvalidHome) {
						t.Errorf("error = %v, want ErrInvalidHome", err)
					}
				})
			}
			if store, err := openQueryHistory(t.Context()); store != nil || !errors.Is(err, config.ErrInvalidHome) {
				if store != nil {
					if err := store.Close(); err != nil {
						t.Error(err)
					}
				}
				t.Errorf("query history = %v, %v, want ErrInvalidHome", store, err)
			}
			if store, closer, err := openHistory(t.Context()); store != nil || closer != nil || !errors.Is(err, config.ErrInvalidHome) {
				if closer != nil {
					if err := closer.Close(); err != nil {
						t.Error(err)
					}
				}
				t.Errorf("history = %v, %v, %v, want ErrInvalidHome", store, closer, err)
			}
			logger := slog.New(slog.DiscardHandler)
			if handled, err := executeNamedQuery(t.Context(), machineFixtureName, logger); !handled || !errors.Is(err, config.ErrInvalidHome) {
				t.Errorf("named query = %v, %v, want handled ErrInvalidHome", handled, err)
			}
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			if handled, err := machineNamedQuery(cmd, parsedFlags{Print: true, Args: []string{machineFixtureName}}); !handled || !errors.Is(err, config.ErrInvalidHome) || cliExitCode(err) != exitProvider {
				t.Errorf("machine named query = %v, %v, want handled ErrInvalidHome with provider exit code", handled, err)
			}
			entries, err := os.ReadDir(project)
			if err != nil || len(entries) != 0 {
				t.Errorf("project storage created: %v, %v", entries, err)
			}
		})
	}
}

// Doctor must report an invalid home without checking project-local storage.
func TestDoctorUntrustedHomeSkipsStorageIO(t *testing.T) {
	for _, home := range []string{"", homeStorageRelativePath} {
		t.Run("home="+home, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("home", home)
			report, cfg := collectDoctorReport(t.Context(), doctorDeps{
				loadConfig: config.Load,
				checkDirectory: func(string) error {
					t.Error("directory check called for untrusted home")
					return nil
				},
				checkHistory: func(context.Context, string) error {
					t.Error("history check called for untrusted home")
					return nil
				},
			})
			if cfg != nil || report.Status != doctorStatusError {
				t.Fatalf("doctor config = %v, status = %s", cfg, report.Status)
			}
			statuses := make(map[string]string)
			for _, check := range report.Checks {
				statuses[check.Name] = check.Status
			}
			if statuses[doctorCheckZevDir] != doctorStatusError || statuses[doctorCheckHistory] != doctorStatusSkipped {
				t.Errorf("doctor storage statuses = %v", statuses)
			}
		})
	}
}
