package main

import (
	"fmt"
	"os"
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
		return err
	}

	fmt.Fprintf(os.Stdout, "✓ Alias %q added successfully\n", name)
	return nil
}

func runAliasList(cmd *cobra.Command, args []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	aliases, err := svc.List()
	if err != nil {
		return err
	}

	if len(aliases) == 0 {
		fmt.Fprintln(os.Stdout, "No aliases configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTEMPLATE")

	for _, alias := range aliases {
		fmt.Fprintf(w, "%s\t%s\n", alias.Name, alias.Template)
	}

	return w.Flush()
}

func runAliasRemove(cmd *cobra.Command, args []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	name := args[0]

	if err := svc.Remove(name); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "✓ Alias %q removed successfully\n", name)
	return nil
}
