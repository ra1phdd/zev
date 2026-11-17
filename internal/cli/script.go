package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
	"github.com/ra1phdd/zev/internal/entities/script/service"
	"github.com/ra1phdd/zev/locale"
	"github.com/spf13/cobra"
)

func newScriptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "script",
		Short: locale.Text(locale.KeyCliManageBashScripts),
		Long:  locale.Text(locale.KeyCliCreateListRemoveBashScripts),
	}

	cmd.AddCommand(newScriptCreateCmd())
	cmd.AddCommand(newScriptListCmd())
	cmd.AddCommand(newScriptShowCmd())
	cmd.AddCommand(newScriptEditCmd())
	cmd.AddCommand(newScriptRemoveCmd())

	return cmd
}

func newScriptCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name> [description]",
		Short: locale.Text(locale.KeyCliCreateNewScriptOpenEditor),
		Long:  locale.Text(locale.KeyCliCreateNewBashScriptZevScriptsOpen),
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runScriptCreate,
	}
}

func newScriptListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: locale.Text(locale.KeyCliListAllScripts),
		RunE:  runScriptList,
	}
}

func newScriptShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: locale.Text(locale.KeyCliShowScriptBody),
		Args:  cobra.ExactArgs(1),
		RunE:  runScriptShow,
	}
}

func runScriptShow(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	repo := repository.New(dir)
	svc := service.New(repo)

	name := args[0]
	if err := svc.Show(cmd.Context(), name, cmd.OutOrStdout()); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliShowScriptError), name, err)
	}
	return nil
}

func newScriptEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: locale.Text(locale.KeyCliEditBashScriptConfiguredEditor),
		Args:  cobra.ExactArgs(1),
		RunE:  runScriptEdit,
	}
}

func runScriptEdit(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	if err := service.New(repository.New(dir)).OpenEditor(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliEditScriptError), args[0], err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliScriptQUpdatedSuccessfully), args[0]); err != nil {
		return fmt.Errorf("write script confirmation: %w", err)
	}
	return nil
}

func newScriptRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   locale.Text(locale.KeyCliRemoveScript),
		Args:    cobra.ExactArgs(1),
		RunE:    runScriptRemove,
	}
}

func runScriptCreate(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	repo := repository.New(dir)
	svc := service.New(repo)

	name := args[0]
	description := ""
	if len(args) > 1 {
		description = args[1]
	}

	if err := svc.CreateContext(cmd.Context(), name, description); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliCreateScriptError), name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliScriptQCreatedSuccessfully), name); err != nil {
		return fmt.Errorf("write script confirmation: %w", err)
	}
	return nil
}

func runScriptList(cmd *cobra.Command, _ []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	repo := repository.New(dir)
	svc := service.New(repo)

	scripts, err := svc.List()
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliListScriptsError), err)
	}

	if len(scripts) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), locale.Text(locale.KeyCliNoScriptsConfigured)); err != nil {
			return fmt.Errorf("write empty script list: %w", err)
		}
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, locale.Text(locale.KeyCliNameFileDescription)); err != nil {
		return fmt.Errorf("write script table header: %w", err)
	}

	for _, script := range scripts {
		if _, err := fmt.Fprintf(w, "%s	%s	%s\n", script.Name, script.ScriptFile, script.Description); err != nil {
			return fmt.Errorf("write script %q: %w", script.Name, err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush script table: %w", err)
	}
	return nil
}

func runScriptRemove(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	repo := repository.New(dir)
	svc := service.New(repo)

	name := args[0]

	if err := svc.Remove(name); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliRemoveScriptError), name, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyCliScriptQRemovedSuccessfully), name); err != nil {
		return fmt.Errorf("write script confirmation: %w", err)
	}
	return nil
}
