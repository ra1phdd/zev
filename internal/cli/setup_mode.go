package cli

import (
	"fmt"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
)

const (
	executionModeInject  = "inject"
	executionModeExecute = "execute"
)

func newSetupModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mode [inject|execute]",
		Short: locale.Text(locale.KeyCliConfigureCommandExecutionMode),
		Long:  locale.Text(locale.KeyCliConfigureZevExecutesSelectedCommandsInjectInsert),
		Args:  cobra.MaximumNArgs(1),
		RunE:  runSetupMode,
	}
}

func runSetupMode(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliFailedLoadConfigError), err)
	}

	if len(args) == 0 {
		mode := cfg.CommandExecutionMode
		if mode == "" {
			mode = executionModeInject
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliCurrentCommandExecutionModeMessage), mode); err != nil {
			return fmt.Errorf("write current execution mode: %w", err)
		}
		return nil
	}

	mode := args[0]

	if mode != executionModeInject && mode != executionModeExecute {
		return fmt.Errorf(locale.Text(locale.KeyCliInvalidModeQMustInjectExecute), mode)
	}

	cfg.CommandExecutionMode = mode

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliFailedSaveConfigError), err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliCommandExecutionModeSetMessage), mode); err != nil {
		return fmt.Errorf("write execution mode confirmation: %w", err)
	}
	return nil
}
