package cli

import (
	"context"
	"fmt"
	"strings"
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

type branchUpdateRunner interface {
	UpdateBranch(context.Context, updateservice.Branch) (updateservice.Result, error)
}

func newUpdateCmd(buildVersion string) *cobra.Command {
	var migrateOnly bool
	cmd := &cobra.Command{
		Use:   "update [stable|main]",
		Short: locale.Text(locale.KeyUpdateShort),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 || len(args) == 1 && args[0] != string(updateservice.Stable) && args[0] != string(updateservice.Main) {
				return fmt.Errorf("update branch must be %q or %q", updateservice.Stable, updateservice.Main)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if migrateOnly {
				ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
				defer cancel()
				if err := runUpdateMigrations(ctx, cmd.ErrOrStderr()); err != nil {
					return err
				}
				return writeQueryOutput(cmd.OutOrStdout(), "%s", locale.Text(locale.KeyUpdateMigrationsDone))
			}
			client, err := updateclient.NewWithProxy(commandProxy(cmd))
			if err != nil {
				return fmt.Errorf("configure update proxy: %w", err)
			}
			updater := updateservice.New(updateservice.Params{
				CurrentVersion: buildVersion,
				CurrentBranch:  branchFromVersion(buildVersion),
				Releases:       client,
				Main:           client,
				Installation:   updaterepo.New(buildVersion),
			})
			branch := updateservice.Stable
			if len(args) == 1 {
				branch = updateservice.Branch(args[0])
			}
			return runBranchUpdate(cmd, updater, branch, branchFromVersion(buildVersion))
		},
	}
	cmd.Flags().BoolVar(&migrateOnly, "migrate-only", false, locale.Text(locale.KeyUpdateMigrateOnly))
	cmd.Flags().Lookup("migrate-only").Hidden = true
	return cmd
}

func branchFromVersion(buildVersion string) updateservice.Branch {
	if strings.HasPrefix(buildVersion, "main-") {
		return updateservice.Main
	}
	return updateservice.Stable
}

func runUpdate(cmd *cobra.Command, updater updateRunner) error {
	return runUpdateWithBranch(cmd, func(ctx context.Context, _ updateservice.Branch) (updateservice.Result, error) {
		return updater.Update(ctx)
	}, updateservice.Stable, updateservice.Stable)
}

func runBranchUpdate(cmd *cobra.Command, updater branchUpdateRunner, branch, currentBranch updateservice.Branch) error {
	return runUpdateWithBranch(cmd, updater.UpdateBranch, branch, currentBranch)
}

func runUpdateWithBranch(cmd *cobra.Command, update func(context.Context, updateservice.Branch) (updateservice.Result, error), branch, currentBranch updateservice.Branch) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
	defer cancel()
	if branch == updateservice.Main {
		if err := writeQueryOutput(cmd.ErrOrStderr(), "%s", locale.Text(locale.KeyUpdateMainWarning)); err != nil {
			return err
		}
	}
	checkingKey := locale.KeyUpdateChecking
	if branch == updateservice.Main {
		checkingKey = locale.KeyUpdateCheckingMain
	}
	if err := writeQueryOutput(cmd.ErrOrStderr(), "%s", locale.Text(checkingKey)); err != nil {
		return err
	}
	result, updateErr := update(ctx, branch)
	if currentBranch == updateservice.Main && branch == updateservice.Stable {
		if err := writeQueryOutput(cmd.ErrOrStderr(), "%s", locale.Text(locale.KeyUpdateMainStableCompatibilityWarning)); err != nil {
			return err
		}
	}
	if updateErr != nil {
		return fmt.Errorf("update Zev: %w", updateErr)
	}
	key := locale.KeyUpdateCurrent
	if result.Updated {
		key = locale.KeyUpdateInstalled
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(key), result.Version)
}
