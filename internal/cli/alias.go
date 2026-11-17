package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/spf13/cobra"
)

func newAliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: locale.Text(locale.KeyCliManageCommandAliases),
		Long:  locale.Text(locale.KeyCliCreateListRemoveCommandAliasesArgumentSubstitution),
	}

	cmd.AddCommand(newAliasAddCmd())
	cmd.AddCommand(newAliasEditCmd())
	cmd.AddCommand(newAliasLastCmd())
	cmd.AddCommand(newAliasListCmd())
	cmd.AddCommand(newAliasRemoveCmd())

	return cmd
}

func newAliasAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <template>",
		Short: locale.Text(locale.KeyCliAddNewAlias),
		Long:  locale.Text(locale.KeyCliAddNewCommandAliasArgumentSubstitutionTemplate),
		Args:  cobra.ExactArgs(2),
		RunE:  runAliasAdd,
	}
}

func newAliasEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: locale.Text(locale.KeyCliEditAliasConfiguredEditor),
		Args:  cobra.ExactArgs(1),
		RunE:  runAliasEdit,
	}
}

func runAliasEdit(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	if err := aliasservice.New(aliasrepo.New(dir)).Edit(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliEditAliasError), args[0], err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliAliasQUpdatedSuccessfully), args[0]); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}

func newAliasLastCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "last <alias-name>",
		Short: locale.Text(locale.KeyCliSaveLastUsedHistoryCommandAlias),
		Args:  cobra.ExactArgs(1),
		RunE:  runAliasLast,
	}
}

func runAliasLast(cmd *cobra.Command, args []string) (err error) {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	store, err := openHistoryStore(cmd.Context(), filepath.Join(dir, "history.db"))
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliOpenHistoryError), err)
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	last, err := store.LastUsedCommand(cmd.Context())
	if errors.Is(err, historyservice.ErrNotFound) {
		return errors.New(locale.Text(locale.KeyCliNoUsableSelectedCommandFoundZevHistory))
	}
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliReadLastHistoryCommandError), err)
	}
	if err := aliasservice.New(aliasrepo.New(dir)).AddCommand(args[0], last.Command); err != nil {
		if errors.Is(err, aliasservice.ErrAliasExists) {
			return fmt.Errorf(locale.Text(locale.KeyCliAliasQAlreadyExistsChooseDifferentName), args[0], err)
		}
		return fmt.Errorf(locale.Text(locale.KeyCliSaveAliasError), args[0], err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliSavedCommandQAliasQRunZev), last.Command, args[0], args[0]); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}

func newAliasListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: locale.Text(locale.KeyCliListAllAliases),
		RunE:  runAliasList,
	}
}

func newAliasRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   locale.Text(locale.KeyCliRemoveAlias),
		Args:    cobra.ExactArgs(1),
		RunE:    runAliasRemove,
	}
}

func runAliasAdd(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	repo := aliasrepo.New(dir)
	svc := aliasservice.New(repo)

	name := args[0]
	template := args[1]

	if err := svc.Add(name, template); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliAddAliasError), name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliAliasQAddedSuccessfully), name); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}

func runAliasList(cmd *cobra.Command, _ []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	repo := aliasrepo.New(dir)
	svc := aliasservice.New(repo)

	aliases, err := svc.List()
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliListAliasesError), err)
	}

	if len(aliases) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), locale.Text(locale.KeyCliNoAliasesConfigured)); err != nil {
			return fmt.Errorf("write empty alias list: %w", err)
		}
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, locale.Text(locale.KeyCliNameTemplate)); err != nil {
		return fmt.Errorf("write alias table header: %w", err)
	}

	for _, alias := range aliases {
		if _, err := fmt.Fprintf(w, "%s	%s\n", alias.Name, alias.Template); err != nil {
			return fmt.Errorf("write alias %q: %w", alias.Name, err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush alias table: %w", err)
	}
	return nil
}

func runAliasRemove(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	repo := aliasrepo.New(dir)
	svc := aliasservice.New(repo)

	name := args[0]

	if err := svc.Remove(name); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliRemoveAliasError), name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliAliasQRemovedSuccessfully), name); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}
