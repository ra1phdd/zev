package main

import (
	"fmt"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/spf13/cobra"
)

const (
	executionModeInject  = "inject"
	executionModeExecute = "execute"
)

func newSetupModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mode [inject|execute]",
		Short: "Configure command execution mode",
		Long: `Configure how zev executes selected commands:
  inject  - Insert command into terminal (default)
  execute - Execute command immediately

Examples:
  zev setup mode inject     # Set mode to inject
  zev setup mode execute    # Set mode to execute
  zev setup mode            # Show current mode`,
		Args: cobra.MaximumNArgs(1),
		RunE: runSetupMode,
	}
}

func runSetupMode(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(args) == 0 {
		mode := cfg.CommandExecutionMode
		if mode == "" {
			mode = executionModeInject
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Current command execution mode: %s\n", mode); err != nil {
			return fmt.Errorf("write current execution mode: %w", err)
		}
		return nil
	}

	mode := args[0]

	if mode != executionModeInject && mode != executionModeExecute {
		return fmt.Errorf("invalid mode %q: must be 'inject' or 'execute'", mode)
	}

	cfg.CommandExecutionMode = mode

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "✓ Command execution mode set to: %s\n", mode); err != nil {
		return fmt.Errorf("write execution mode confirmation: %w", err)
	}
	return nil
}
