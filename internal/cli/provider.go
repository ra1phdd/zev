package cli

import (
	"fmt"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	setupservice "github.com/ra1phdd/zev/internal/entities/setup/service"
	configmigration "github.com/ra1phdd/zev/internal/migrations/config"
	"github.com/spf13/cobra"
)

func newProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: locale.Text(locale.KeyProviderShort),
	}
	cmd.AddCommand(newProviderListCmd(), newProviderUseCmd(), newProviderAddCmd(), newProviderRemoveCmd())
	return cmd
}

func newProviderListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: locale.Text(locale.KeyProviderListShort),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd.Context())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			names := cfg.ProviderNames()
			if len(names) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), locale.Text(locale.KeyProviderListEmpty)); err != nil {
					return fmt.Errorf("write provider list: %w", err)
				}
				return nil
			}
			for _, name := range names {
				profile := cfg.Providers[name]
				marker := " "
				if name == cfg.ActiveProvider {
					marker = "*"
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf(locale.Text(locale.KeyProviderListEntry), marker, name, profile.Type)); err != nil {
					return fmt.Errorf("write provider list: %w", err)
				}
			}
			return nil
		},
	}
}

func newProviderUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: locale.Text(locale.KeyProviderUseShort),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := prepareStore(cmd.Context(), configmigration.Migrate); err != nil {
				return fmt.Errorf("migrate configuration: %w", err)
			}
			if err := config.UpdateContext(cmd.Context(), func(cfg *config.Config) error {
				if _, ok := cfg.Providers[name]; !ok {
					return fmt.Errorf(locale.Text(locale.KeyProviderMissing), name)
				}
				cfg.ActiveProvider = name
				return nil
			}); err != nil {
				return fmt.Errorf("select provider: %w", err)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf(locale.Text(locale.KeyProviderUseSuccess), name)); err != nil {
				return fmt.Errorf("write provider confirmation: %w", err)
			}
			return nil
		},
	}
}

func newProviderAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add NAME",
		Short: locale.Text(locale.KeyProviderAddShort),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := prepareStore(cmd.Context(), configmigration.Migrate); err != nil {
				return fmt.Errorf("migrate configuration: %w", err)
			}
			if err := setupservice.New().RunNamedContext(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("add provider: %w", err)
			}
			return nil
		},
	}
}

func newProviderRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: locale.Text(locale.KeyProviderRemoveShort),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := prepareStore(cmd.Context(), configmigration.Migrate); err != nil {
				return fmt.Errorf("migrate configuration: %w", err)
			}
			if err := config.UpdateContext(cmd.Context(), func(cfg *config.Config) error {
				if _, ok := cfg.Providers[name]; !ok {
					return fmt.Errorf(locale.Text(locale.KeyProviderMissing), name)
				}
				if cfg.ActiveProvider == name {
					return fmt.Errorf(locale.Text(locale.KeyProviderActiveRemoval), name)
				}
				delete(cfg.Providers, name)
				return nil
			}); err != nil {
				return fmt.Errorf("remove provider: %w", err)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf(locale.Text(locale.KeyProviderRemoveSuccess), name)); err != nil {
				return fmt.Errorf("write provider confirmation: %w", err)
			}
			return nil
		},
	}
}
