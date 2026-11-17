// Package cli provides Zev command routing and terminal presentation.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/locale"
	"github.com/spf13/cobra"
)

var version = "dev"

var verbosityLevel int

// NewRootCommand creates the command tree for the supplied build version.
func NewRootCommand(buildVersion string) *cobra.Command {
	version = buildVersion
	if configured, err := config.LoadUILanguage(); err == nil {
		defer locale.SetLanguage(configured, "")()
	}
	rootCmd := &cobra.Command{
		Use:                "zev [query]",
		Short:              locale.Text(locale.KeyQueryNaturalLanguageTerminalCommandSuggestions),
		Long:               locale.Text(locale.KeyQueryZevHelpsYouRememberTerminalCommandsDescribing),
		Args:               cobra.ArbitraryArgs,
		RunE:               runQuery,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: true, // Preserve flags intended for aliases.
	}
	// Find preserves pre-subcommand flags for the query's manual parser; Traverse
	// would discard them when DisableFlagParsing is set on the root.
	rootCmd.TraverseChildren = false
	registerToolFlags(rootCmd)

	rootCmd.PersistentFlags().CountVarP(&verbosityLevel, "verbose", "v", locale.Text(locale.KeyQueryIncreaseVerbosityLevelVInfoVvDebug))

	rootCmd.AddCommand(newQueryCmd())
	rootCmd.AddCommand(newContinueCmd())
	rootCmd.AddCommand(newHistoryCmd())
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newAliasCmd())
	rootCmd.AddCommand(newScriptCmd())
	rootCmd.AddCommand(newVersionCmd())
	localizeCommandHelp(rootCmd)
	return rootCmd
}

// Execute runs a configured command tree and returns its process exit code.
func Execute(ctx context.Context, root *cobra.Command) int {
	if configured, err := config.LoadUILanguage(); err == nil {
		defer locale.SetLanguage(configured, "")()
	}
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	if machineErr, ok := errors.AsType[*machineExitError](err); ok {
		if _, writeErr := fmt.Fprintf(os.Stderr, locale.Text(locale.KeyQueryErrorMessage), err); writeErr != nil {
			slog.Error("Failed to write command error", "error", writeErr)
		}
		return machineErr.code
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode()
	}
	if _, writeErr := fmt.Fprintf(os.Stderr, locale.Text(locale.KeyQueryErrorMessage), err); writeErr != nil {
		slog.Error("Failed to write command error", "error", writeErr)
	}
	return 1
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: locale.Text(locale.KeyQueryShowZevVersion),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyQueryZevVersionMessage), version); err != nil {
				return fmt.Errorf("write version: %w", err)
			}
			return nil
		},
	}
}
