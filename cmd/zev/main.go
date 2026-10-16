package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var verbosityLevel int

func main() {
	rootCmd := &cobra.Command{
		Use:           "zev [query]",
		Short:         "Natural language terminal command suggestions",
		Long:          "Zev helps you remember terminal commands by describing what you want to do in natural language.",
		Args:          cobra.ArbitraryArgs,
		RunE:          runQuery,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().CountVarP(&verbosityLevel, "verbose", "v", "Increase verbosity level (-v: INFO, -vv: DEBUG, -vvv: TRACE)")

	rootCmd.AddCommand(newQueryCmd())
	rootCmd.AddCommand(newHistoryCmd())
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show zev version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(os.Stdout, "zev version %s\n", version)
		},
	}
}
