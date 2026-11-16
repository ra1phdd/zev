// Package main provides the Zev command-line application.
package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
	"github.com/ra1phdd/zev/internal/entities/alias/service"
	"github.com/spf13/cobra"
)

func newAliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage command aliases",
		Long:  "Create, list, and remove command aliases with argument substitution",
	}

	cmd.AddCommand(newAliasAddCmd())
	cmd.AddCommand(newAliasListCmd())
	cmd.AddCommand(newAliasRemoveCmd())

	return cmd
}

func newAliasAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <template>",
		Short: "Add a new alias",
		Long: `Add a new command alias with argument substitution.

Template variables:
  {{.arg1}}, {{.arg2}}, ... - positional arguments
  {{.args}}                  - all arguments joined with spaces

Example:
  zev alias add dps "docker service ps {{.arg1}} --no-trunc"`,
		Args: cobra.ExactArgs(2),
		RunE: runAliasAdd,
	}
}

func newAliasListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all aliases",
		RunE:  runAliasList,
	}
}

func newAliasRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove an alias",
		Args:    cobra.ExactArgs(1),
		RunE:    runAliasRemove,
	}
}

func runAliasAdd(cmd *cobra.Command, args []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	name := args[0]
	template := args[1]

	if err := svc.Add(name, template); err != nil {
		return fmt.Errorf("add alias %q: %w", name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "✓ Alias %q added successfully\n", name); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}

func runAliasList(cmd *cobra.Command, _ []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	aliases, err := svc.List()
	if err != nil {
		return fmt.Errorf("list aliases: %w", err)
	}

	if len(aliases) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "No aliases configured"); err != nil {
			return fmt.Errorf("write empty alias list: %w", err)
		}
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME	TEMPLATE"); err != nil {
		return fmt.Errorf("write alias table header: %w", err)
	}

	for _, alias := range aliases {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", alias.Name, alias.Template); err != nil {
			return fmt.Errorf("write alias %q: %w", alias.Name, err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush alias table: %w", err)
	}
	return nil
}

func runAliasRemove(cmd *cobra.Command, args []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	name := args[0]

	if err := svc.Remove(name); err != nil {
		return fmt.Errorf("remove alias %q: %w", name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "✓ Alias %q removed successfully\n", name); err != nil {
		return fmt.Errorf("write alias confirmation: %w", err)
	}
	return nil
}
