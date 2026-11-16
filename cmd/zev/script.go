package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
	"github.com/ra1phdd/zev/internal/entities/script/service"
	"github.com/spf13/cobra"
)

func newScriptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "script",
		Short: "Manage bash scripts",
		Long:  "Create, list, and remove bash scripts",
	}

	cmd.AddCommand(newScriptCreateCmd())
	cmd.AddCommand(newScriptListCmd())
	cmd.AddCommand(newScriptRemoveCmd())

	return cmd
}

func newScriptCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name> [description]",
		Short: "Create a new script and open it in $EDITOR",
		Long: `Create a new bash script in ~/.zev/scripts/ and open it in $EDITOR.

Example:
  zev script create deploy "Deploy application to production"`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runScriptCreate,
	}
}

func newScriptListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all scripts",
		RunE:  runScriptList,
	}
}

func newScriptRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a script",
		Args:    cobra.ExactArgs(1),
		RunE:    runScriptRemove,
	}
}

func runScriptCreate(cmd *cobra.Command, args []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	name := args[0]
	description := ""
	if len(args) > 1 {
		description = args[1]
	}

	if err := svc.CreateContext(cmd.Context(), name, description); err != nil {
		return fmt.Errorf("create script %q: %w", name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "✓ Script %q created successfully\n", name); err != nil {
		return fmt.Errorf("write script confirmation: %w", err)
	}
	return nil
}

func runScriptList(cmd *cobra.Command, _ []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	scripts, err := svc.List()
	if err != nil {
		return fmt.Errorf("list scripts: %w", err)
	}

	if len(scripts) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "No scripts configured"); err != nil {
			return fmt.Errorf("write empty script list: %w", err)
		}
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME	FILE	DESCRIPTION"); err != nil {
		return fmt.Errorf("write script table header: %w", err)
	}

	for _, script := range scripts {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", script.Name, script.ScriptFile, script.Description); err != nil {
			return fmt.Errorf("write script %q: %w", script.Name, err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush script table: %w", err)
	}
	return nil
}

func runScriptRemove(cmd *cobra.Command, args []string) error {
	repo := repository.New(config.GetZevDir())
	svc := service.New(repo)

	name := args[0]

	if err := svc.Remove(name); err != nil {
		return fmt.Errorf("remove script %q: %w", name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "✓ Script %q removed successfully\n", name); err != nil {
		return fmt.Errorf("write script confirmation: %w", err)
	}
	return nil
}
