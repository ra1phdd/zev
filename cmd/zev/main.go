package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var version = "dev"

var verbosityLevel int

func main() {
	rootCmd := &cobra.Command{
		Use:                "zev [query]",
		Short:              "Natural language terminal command suggestions",
		Long:               "Zev helps you remember terminal commands by describing what you want to do in natural language.",
		Args:               cobra.ArbitraryArgs,
		RunE:               runQuery,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: true, // Preserve flags intended for aliases.
	}
	rootCmd.TraverseChildren = true

	rootCmd.PersistentFlags().CountVarP(&verbosityLevel, "verbose", "v", "Increase verbosity level (-v: INFO, -vv: DEBUG, -vvv: TRACE)")

	rootCmd.AddCommand(newQueryCmd())
	rootCmd.AddCommand(newHistoryCmd())
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newAliasCmd())
	rootCmd.AddCommand(newScriptCmd())
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		// Preserve child exit codes without adding another diagnostic.
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			os.Exit(exitErr.ExitCode())
		}
		if _, writeErr := fmt.Fprintf(os.Stderr, "Error: %v\n", err); writeErr != nil {
			slog.Error("Failed to write command error", "error", writeErr)
		}
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show zev version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "zev version %s\n", version); err != nil {
				return fmt.Errorf("write version: %w", err)
			}
			return nil
		},
	}
}
