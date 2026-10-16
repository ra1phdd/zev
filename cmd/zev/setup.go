package main

import (
	setupservice "github.com/ra1phdd/zev/internal/entities/setup/service"
	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure zev with your LLM provider credentials",
		RunE:  runSetup,
	}

	setupCmd.AddCommand(newSetupHeadersCmd())
	setupCmd.AddCommand(newSetupModeCmd())

	return setupCmd
}

func runSetup(cmd *cobra.Command, args []string) error {
	setupSvc := setupservice.New()
	return setupSvc.Run()
}
