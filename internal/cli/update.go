package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/ra1phdd/zev/internal/core/locale"
	updateclient "github.com/ra1phdd/zev/internal/entities/update/client"
	updaterepo "github.com/ra1phdd/zev/internal/entities/update/repository"
	updateservice "github.com/ra1phdd/zev/internal/entities/update/service"
	"github.com/spf13/cobra"
)

type updateRunner interface {
	Update(context.Context) (updateservice.Result, error)
}

func newUpdateCmd(buildVersion string) *cobra.Command {
	var migrateOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: locale.Text(locale.KeyUpdateShort),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if migrateOnly {
				ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
				defer cancel()
				if err := runUpdateMigrations(ctx, cmd.ErrOrStderr()); err != nil {
					return err
				}
				return writeQueryOutput(cmd.OutOrStdout(), "%s", locale.Text(locale.KeyUpdateMigrationsDone))
			}
			updater := updateservice.New(updateservice.Params{
				CurrentVersion: buildVersion,
				Releases:       updateclient.New(),
				Installation:   updaterepo.New(buildVersion),
			})
			return runUpdate(cmd, updater)
		},
	}
	cmd.Flags().BoolVar(&migrateOnly, "migrate-only", false, locale.Text(locale.KeyUpdateMigrateOnly))
	cmd.Flags().Lookup("migrate-only").Hidden = true
	return cmd
}

func runUpdate(cmd *cobra.Command, updater updateRunner) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
	defer cancel()
	if err := writeQueryOutput(cmd.ErrOrStderr(), "%s", locale.Text(locale.KeyUpdateChecking)); err != nil {
		return err
	}
	result, err := updater.Update(ctx)
	if err != nil {
		return fmt.Errorf("update Zev: %w", err)
	}
	key := locale.KeyUpdateCurrent
	if result.Updated {
		key = locale.KeyUpdateInstalled
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(key), result.Version)
}
