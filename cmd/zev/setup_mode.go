package main

import (
	"fmt"
	"os"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/spf13/cobra"
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

	// Если аргумент не передан — показываем текущий режим
	if len(args) == 0 {
		mode := cfg.CommandExecutionMode
		if mode == "" {
			mode = "inject" // дефолт
		}
		fmt.Fprintf(os.Stdout, "Current command execution mode: %s\n", mode)
		return nil
	}

	mode := args[0]

	// Валидация
	if mode != "inject" && mode != "execute" {
		return fmt.Errorf("invalid mode %q: must be 'inject' or 'execute'", mode)
	}

	cfg.CommandExecutionMode = mode

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(os.Stdout, "✓ Command execution mode set to: %s\n", mode)
	return nil
}
