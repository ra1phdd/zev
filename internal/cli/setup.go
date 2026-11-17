package cli

import (
	"fmt"

	"github.com/ra1phdd/zev/internal/core/locale"
	setupservice "github.com/ra1phdd/zev/internal/entities/setup/service"
	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: locale.Text(locale.KeyCliConfigureZevLlmProviderCredentials),
		RunE:  runSetup,
	}

	setupCmd.AddCommand(newSetupHeadersCmd())
	setupCmd.AddCommand(newSetupModeCmd())

	return setupCmd
}

func runSetup(cmd *cobra.Command, _ []string) error {
	setupSvc := setupservice.New()
	if err := setupSvc.RunContext(cmd.Context()); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliRunSetupError), err)
	}
	return nil
}
