package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/spf13/cobra"
)

func TestRootCommandPreservesToolPolicyBeforeSubcommand(t *testing.T) {
	// Root construction binds existing package-level Cobra flags.
	t.Setenv("ZEV_LANG", "en")
	for _, tc := range []struct {
		name        string
		args        []string
		wantCommand string
		enabled     bool
		mode        string
	}{
		{name: "no tools before query", args: []string{toolDisableFlag, historyValidationQuery, toolTestPrompt}, wantCommand: historyValidationQuery, mode: toolModeBypass},
		{name: "manual before query", args: []string{"--tool-approval=manual", historyValidationQuery, toolTestPrompt}, wantCommand: historyValidationQuery, enabled: true, mode: toolModeManual},
		{name: "separate mode value", args: []string{toolApprovalFlag, toolModeManual, historyValidationQuery, toolTestPrompt}, wantCommand: historyValidationQuery, enabled: true, mode: toolModeManual},
		{name: "continue override", args: []string{toolDisableFlag, "continue", toolTestPrompt}, wantCommand: "continue", mode: toolModeBypass},
		{name: "after subcommand", args: []string{historyValidationQuery, toolDisableFlag, toolTestPrompt}, wantCommand: historyValidationQuery, mode: toolModeBypass},
		{name: "explicit machine tools", args: []string{toolEnableFlag, historyValidationQuery, machinePrintFlag, toolTestPrompt}, wantCommand: historyValidationQuery, enabled: true, mode: toolModeBypass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand("dev")
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			called := false
			for _, cmd := range root.Commands() {
				if cmd.Name() != tc.wantCommand {
					continue
				}
				cmd.RunE = func(cmd *cobra.Command, args []string) error {
					called = true
					cfg := config.DefaultConfig()
					cfg.ToolApprovalMode = toolModeBypass
					flags := parseArgsFlags(args)
					if err := validateMachineFlags(flags); err != nil {
						return err
					}
					enabled, mode := resolveToolSettings(cfg, flags)
					if enabled != tc.enabled || mode != tc.mode {
						t.Errorf("policy lost: command=%s args=%q enabled=%t mode=%s", cmd.Name(), args, enabled, mode)
					}
					if strings.Join(flags.Args, " ") != toolTestPrompt {
						t.Errorf("query arguments changed %q", flags.Args)
					}
					return nil
				}
			}
			root.SetArgs(tc.args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("expected subcommand not reached")
			}
		})
	}
}
