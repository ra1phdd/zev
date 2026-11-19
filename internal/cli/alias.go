package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/metadata"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	aliasmigration "github.com/ra1phdd/zev/internal/migrations/aliases"
	"github.com/spf13/cobra"
)

func newAliasCmd() *cobra.Command {
	cmd := &cobra.Command{Use: managedKindAlias, Short: locale.Text(locale.KeyCliManageCommandAliases), Long: locale.Text(locale.KeyCliCreateListRemoveCommandAliasesArgumentSubstitution)}
	cmd.AddCommand(newAliasAddCmd(), newAliasEditCmd(), newAliasLastCmd(), newAliasListCmd(), newAliasRemoveCmd())
	return cmd
}

func newAliasAddCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "add [name template [description [arguments-json]]]", Short: locale.Text(locale.KeyCliAddNewAlias), Long: locale.Text(locale.KeyCliAddNewCommandAliasArgumentSubstitutionTemplate), Args: managedRangeArgs(0, 4), RunE: runAliasAdd}
	addManagedFlags(cmd, managedKindAlias, managedOpAdd)
	return cmd
}

func newAliasEditCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "edit [name]", Short: locale.Text(locale.KeyCliEditAliasConfiguredEditor), Args: managedRangeArgs(0, 1), RunE: runAliasEdit}
	addManagedFlags(cmd, managedKindAlias, managedOpEdit)
	return cmd
}

func newAliasListCmd() *cobra.Command {
	cmd := &cobra.Command{Use: managedOpList, Short: locale.Text(locale.KeyCliListAllAliases), Args: managedRangeArgs(0, 0), RunE: runAliasList}
	addManagedFlags(cmd, managedKindAlias, managedOpList)
	return cmd
}

func newAliasRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "remove [name]", Aliases: []string{"rm", "delete"}, Short: locale.Text(locale.KeyCliRemoveAlias), Args: managedRangeArgs(0, 1), RunE: runAliasRemove}
	addManagedFlags(cmd, managedKindAlias, managedOpRemove)
	return cmd
}

func newAliasLastCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "last <alias-name>", Short: locale.Text(locale.KeyCliSaveLastUsedHistoryCommandAlias), Args: managedRangeArgs(1, 1), RunE: runAliasLast}
	addManagedFlags(cmd, managedKindAlias, "last")
	return cmd
}

//nolint:gocognit // The add mode matrix validates every field before publishing.
func runAliasAdd(cmd *cobra.Command, args []string) error {
	if _, err := config.GetZevDir(); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	p, err := readManagedFields(cmd, args, managedKindAlias, managedOpAdd)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	var a domain.Alias
	var form *managedForm
	//nolint:nestif // The interactive form completes before any registration.
	if len(args) == 0 {
		if err := requireManagedInteractive(cmd, p); err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		form = newManagedForm(cmd)
		a.Name, err = form.field(locale.KeyManagedName, "")
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		if p.template != nil {
			a.Template = *p.template
		} else {
			a.Template, err = form.field(locale.KeyManagedTemplate, "")
			if err != nil {
				return fmt.Errorf("read alias template: %w", err)
			}
		}
		if !p.noMetadata {
			a.Description, a.Arguments, err = form.creationMetadata(p)
			if err != nil {
				return fmt.Errorf("manage entity: %w", err)
			}
		}
	} else {
		a.Name = args[0]
		if p.template == nil {
			return usageError(locale.KeyManagedTTYRequired)
		}
		a.Template = *p.template
	}
	if p.description != nil {
		a.Description = *p.description
	}
	if p.arguments != nil {
		a.Arguments = *p.arguments
	}
	if err := generateManagedMetadata(cmd, managedGeneration{Alias: &a, Description: p.generateDescription, Arguments: p.generateArguments, Interactive: len(args) == 0, Form: form}); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	if err := metadata.ValidateAlias(a); err != nil {
		return machineError(exitArguments, err)
	}
	dir, err := prepareStore(cmd.Context(), aliasmigration.Migrate)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	if err := createManagedEntity(cmd.Context(), cmd, dir, managedKindAlias, a.Name, func() error {
		return aliasservice.New(aliasrepo.New(dir)).AddAlias(a)
	}); err != nil {
		return managedStorageError(err)
	}
	if p.json {
		record := aliasRecord(a)
		return writeManagedJSON(cmd, managedKindAlias, &record, nil)
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyCliAliasQAddedSuccessfully), a.Name)
}

//nolint:gocognit // Field-level patch orchestration keeps prompting before the locked update.
func runAliasEdit(cmd *cobra.Command, args []string) error {
	if _, err := config.GetZevDir(); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	p, err := readManagedFields(cmd, args, managedKindAlias, managedOpEdit)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	interactive := p.template == nil && p.description == nil && p.arguments == nil && !p.generateDescription && !p.generateArguments
	if len(args) == 0 || interactive {
		if err := requireManagedInteractive(cmd, p); err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	dir, err := prepareStore(cmd.Context(), aliasmigration.Migrate)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	svc := aliasservice.New(aliasrepo.New(dir))
	all, err := svc.List()
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	form := newManagedForm(cmd)
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		names := make([]string, len(all))
		for i, a := range all {
			names[i] = a.Name
		}
		name, err = form.selectName(names)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	var snapshot domain.Alias
	found := false
	for _, a := range all {
		if a.Name == name {
			snapshot = a
			found = true
			break
		}
	}
	if !found {
		return machineError(exitArguments, fmt.Errorf("%w: %q", aliasservice.ErrAliasNotFound, name))
	}
	candidate := snapshot
	if interactive {
		text, err := form.field(locale.KeyManagedTemplate, snapshot.Template)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		p.template = &text
		desc, arguments, err := form.metadata(snapshot.Description, snapshot.Arguments)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		p.description = &desc
		p.arguments = &arguments
	}
	if p.template != nil {
		candidate.Template = *p.template
	}
	if p.description != nil {
		candidate.Description = *p.description
	}
	if p.arguments != nil {
		candidate.Arguments = *p.arguments
	}
	if err := generateManagedMetadata(cmd, managedGeneration{Alias: &candidate, Description: p.generateDescription, Arguments: p.generateArguments, Interactive: len(args) == 0 || interactive, Form: form}); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	if p.generateDescription {
		p.description = &candidate.Description
	}
	if p.generateArguments {
		p.arguments = &candidate.Arguments
	}
	if err := metadata.ValidateAlias(candidate); err != nil {
		return machineError(exitArguments, err)
	}
	result, err := svc.Patch(snapshot, aliasservice.Patch{Template: p.template, Description: p.description, Arguments: p.arguments})
	if err != nil {
		return managedStorageError(err)
	}
	if p.json {
		record := aliasRecord(result)
		return writeManagedJSON(cmd, managedKindAlias, &record, nil)
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyCliAliasQUpdatedSuccessfully), name)
}

func runAliasRemove(cmd *cobra.Command, args []string) error {
	if _, err := config.GetZevDir(); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	p, err := readManagedFields(cmd, nil, managedKindAlias, managedOpRemove)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	if len(args) == 0 {
		if err := requireManagedInteractive(cmd, p); err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	dir, err := prepareStore(cmd.Context(), aliasmigration.Migrate)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	svc := aliasservice.New(aliasrepo.New(dir))
	all, err := svc.List()
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		form := newManagedForm(cmd)
		names := make([]string, len(all))
		for i, a := range all {
			names[i] = a.Name
		}
		name, err = form.selectName(names)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		yes, err := form.confirm(name)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		if !yes {
			return nil
		}
	}
	var removed domain.Alias
	found := false
	for _, a := range all {
		if a.Name == name {
			removed = a
			found = true
			break
		}
	}
	if !found {
		return machineError(exitArguments, fmt.Errorf("%w: %q", aliasservice.ErrAliasNotFound, name))
	}
	if err := svc.Remove(name); err != nil {
		return managedStorageError(err)
	}
	if p.json {
		record := aliasRecord(removed)
		return writeManagedJSON(cmd, managedKindAlias, &record, nil)
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyCliAliasQRemovedSuccessfully), name)
}

func runAliasList(cmd *cobra.Command, _ []string) error {
	dir, present, err := zevDirectory()
	if err != nil {
		return err
	}
	if !present {
		return writeManagedList(cmd, managedKindAlias, []managedRecord{})
	}
	if err := aliasmigration.Migrate(cmd.Context(), dir); err != nil {
		return fmt.Errorf("migrate alias store: %w", err)
	}
	all, err := aliasservice.New(aliasrepo.New(dir)).List()
	if err != nil {
		return fmt.Errorf("list aliases: %w", err)
	}
	items := make([]managedRecord, len(all))
	for i, a := range all {
		items[i] = aliasRecord(a)
		arguments, err := metadata.EffectiveArguments(a)
		if err != nil {
			return fmt.Errorf("read alias contract: %w", err)
		}
		items[i].usage = entityUsage(a.Name, arguments)
	}
	return writeManagedList(cmd, managedKindAlias, items)
}

func runAliasLast(cmd *cobra.Command, args []string) (err error) {
	p, err := readManagedFields(cmd, args, managedKindAlias, "last")
	if err != nil {
		return fmt.Errorf("read alias metadata: %w", err)
	}
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
	alias := domain.Alias{Name: args[0], Template: last.Command}
	if p.description != nil {
		alias.Description = *p.description
	}
	if p.arguments != nil {
		alias.Arguments = *p.arguments
	}
	if err := generateManagedMetadata(cmd, managedGeneration{Alias: &alias, Description: p.generateDescription, Arguments: p.generateArguments, Interactive: false}); err != nil {
		return fmt.Errorf("generate alias metadata: %w", err)
	}
	if err := metadata.ValidateAlias(alias); err != nil {
		return machineError(exitArguments, err)
	}
	if err := aliasmigration.Migrate(cmd.Context(), dir); err != nil {
		return fmt.Errorf("migrate aliases: %w", err)
	}
	if err := createManagedEntity(cmd.Context(), cmd, dir, managedKindAlias, args[0], func() error {
		return aliasservice.New(aliasrepo.New(dir)).AddAlias(alias)
	}); err != nil {
		if errors.Is(err, aliasservice.ErrAliasExists) {
			return fmt.Errorf(locale.Text(locale.KeyCliAliasQAlreadyExistsChooseDifferentName), args[0], err)
		}
		return managedStorageError(fmt.Errorf(locale.Text(locale.KeyCliSaveAliasError), args[0], err))
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyCliSavedCommandQAliasQRunZev), last.Command, args[0], args[0])
}
