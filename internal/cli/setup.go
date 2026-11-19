package cli

import (
	"fmt"

	"github.com/ra1phdd/zev/internal/core/locale"
	setupservice "github.com/ra1phdd/zev/internal/entities/setup/service"
	configmigration "github.com/ra1phdd/zev/internal/migrations/config"
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
	if _, err := prepareStore(cmd.Context(), configmigration.Migrate); err != nil {
		return fmt.Errorf("migrate configuration: %w", err)
	}
	setupSvc := setupservice.NewWithProxy(commandProxy(cmd))
	if err := setupSvc.RunContext(cmd.Context()); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliRunSetupError), err)
	}
	return nil
}
