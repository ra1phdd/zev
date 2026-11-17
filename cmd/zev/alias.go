// Package main provides the Zev command-line application.
package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/ra1phdd/zev/internal/core/config"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	historyrepo "github.com/ra1phdd/zev/internal/entities/history/repository"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/locale"
	"github.com/spf13/cobra"
)

func newAliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: locale.Text("Manage command aliases"),
		Long:  locale.Text("Create, list, and remove command aliases with argument substitution"),
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
		Short: locale.Text("Add a new alias"),
		Long: locale.Text(`Add a new command alias with argument substitution.

Template variables:
  {{.arg1}}, {{.arg2}}, ... - positional arguments
  {{.args}}                  - all arguments joined with spaces

Example:
  zev alias add dps "docker service ps {{.arg1}} --no-trunc"`),
		Args: cobra.ExactArgs(2),
		RunE: runAliasAdd,
	}
}

func newAliasEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: locale.Text("Edit an alias in the configured editor"),
		Args:  cobra.ExactArgs(1),
		RunE:  runAliasEdit,
	}
}

func runAliasEdit(cmd *cobra.Command, args []string) error {
	if err := aliasservice.New(aliasrepo.New(config.GetZevDir())).Edit(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf(locale.Text("edit alias %q: %w"), args[0], err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text("✓ Alias %q updated successfully\n"), args[0]); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}

func newAliasLastCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "last <alias-name>",
		Short: locale.Text("Save the last used history command as an alias"),
		Args:  cobra.ExactArgs(1),
		RunE:  runAliasLast,
	}
}

func runAliasLast(cmd *cobra.Command, args []string) (err error) {
	dir := config.GetZevDir()
	store, err := historyrepo.Open(cmd.Context(), filepath.Join(dir, "history.db"))
	if err != nil {
		return fmt.Errorf(locale.Text("open history: %w"), err)
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	last, err := store.LastUsedCommand(cmd.Context())
	if errors.Is(err, historyservice.ErrNotFound) {
		return errors.New(locale.Text("no usable selected command found in Zev history"))
	}
	if err != nil {
		return fmt.Errorf(locale.Text("read last history command: %w"), err)
	}
	if err := aliasservice.New(aliasrepo.New(dir)).AddCommand(args[0], last.Command); err != nil {
		if errors.Is(err, aliasservice.ErrAliasExists) {
			return fmt.Errorf(locale.Text("alias %q already exists; choose a different name: %w"), args[0], err)
		}
		return fmt.Errorf(locale.Text("save alias %q: %w"), args[0], err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text("✓ Saved command %q as alias %q\nRun it with: zev %s\n"), last.Command, args[0], args[0]); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}

func newAliasListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: locale.Text("List all aliases"),
		RunE:  runAliasList,
	}
}

func newAliasRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   locale.Text("Remove an alias"),
		Args:    cobra.ExactArgs(1),
		RunE:    runAliasRemove,
	}
}

func runAliasAdd(cmd *cobra.Command, args []string) error {
	repo := aliasrepo.New(config.GetZevDir())
	svc := aliasservice.New(repo)

	name := args[0]
	template := args[1]

	if err := svc.Add(name, template); err != nil {
		return fmt.Errorf(locale.Text("add alias %q: %w"), name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text("✓ Alias %q added successfully\n"), name); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}

func runAliasList(cmd *cobra.Command, _ []string) error {
	repo := aliasrepo.New(config.GetZevDir())
	svc := aliasservice.New(repo)

	aliases, err := svc.List()
	if err != nil {
		return fmt.Errorf(locale.Text("list aliases: %w"), err)
	}

	if len(aliases) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), locale.Text("No aliases configured")); err != nil {
			return fmt.Errorf("write empty alias list: %w", err)
		}
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, locale.Text("NAME	TEMPLATE")); err != nil {
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
	repo := aliasrepo.New(config.GetZevDir())
	svc := aliasservice.New(repo)

	name := args[0]

	if err := svc.Remove(name); err != nil {
		return fmt.Errorf(locale.Text("remove alias %q: %w"), name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text("✓ Alias %q removed successfully\n"), name); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}
