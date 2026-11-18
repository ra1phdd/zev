package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/entities/script/repository"
	"github.com/ra1phdd/zev/internal/entities/script/service"
	"github.com/spf13/cobra"
)

func newScriptCmd() *cobra.Command {
	cmd := &cobra.Command{Use: managedKindScript, Short: locale.Text(locale.KeyCliManageBashScripts), Long: locale.Text(locale.KeyCliCreateListRemoveBashScripts)}
	cmd.AddCommand(newScriptCreateCmd(), newScriptEditCmd(), newScriptListCmd(), newScriptRemoveCmd(), newScriptShowCmd())
	return cmd
}

func newScriptCreateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "create [name [description [arguments-json]]]", Short: locale.Text(locale.KeyCliCreateNewScriptOpenEditor), Long: locale.Text(locale.KeyCliCreateNewBashScriptZevScriptsOpen), Args: managedRangeArgs(0, 3), RunE: runScriptCreate}
	addManagedFlags(cmd, managedKindScript, managedOpCreate)
	return cmd
}

func newScriptEditCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "edit [name]", Short: locale.Text(locale.KeyCliEditBashScriptConfiguredEditor), Args: managedRangeArgs(0, 1), RunE: runScriptEdit}
	addManagedFlags(cmd, managedKindScript, managedOpEdit)
	return cmd
}

func newScriptListCmd() *cobra.Command {
	cmd := &cobra.Command{Use: managedOpList, Short: locale.Text(locale.KeyCliListAllScripts), Args: managedRangeArgs(0, 0), RunE: runScriptList}
	addManagedFlags(cmd, managedKindScript, managedOpList)
	return cmd
}

func newScriptRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "remove [name]", Aliases: []string{"rm"}, Short: locale.Text(locale.KeyCliRemoveScript), Args: managedRangeArgs(0, 1), RunE: runScriptRemove}
	addManagedFlags(cmd, managedKindScript, managedOpRemove)
	return cmd
}

func newScriptShowCmd() *cobra.Command {
	return &cobra.Command{Use: "show <name>", Short: locale.Text(locale.KeyCliShowScriptBody), Args: managedRangeArgs(1, 1), RunE: runScriptShow}
}

func readManagedBody(cmd *cobra.Command, p managedFields) (body []byte, err error) {
	if p.bodyStdin {
		body, err = io.ReadAll(cmd.InOrStdin())
	} else {
		body, err = os.ReadFile(p.bodyFile)
	}
	if err != nil {
		return nil, fmt.Errorf("read script body: %w", err)
	}
	return body, nil
}

//nolint:gocognit // The create mode matrix validates all fields before reading body stdin.
func runScriptCreate(cmd *cobra.Command, args []string) error {
	if _, err := config.GetZevDir(); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	p, err := readManagedFields(cmd, args, managedKindScript, managedOpCreate)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	editor := !p.bodyStdin && p.bodyFile == ""
	if editor || len(args) == 0 {
		if err := requireManagedInteractive(cmd, p); err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	var script domain.Script
	var form *managedForm
	if len(args) > 0 {
		script.Name = args[0]
	} else {
		form = newManagedForm(cmd)
		script.Name, err = form.field(locale.KeyManagedName, "")
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		script.Description, script.Arguments, err = form.creationMetadata(p)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	script.ScriptFile = script.Name + ".sh"
	if p.description != nil {
		script.Description = *p.description
	}
	if p.arguments != nil {
		script.Arguments = *p.arguments
	}
	if err := metadata.ValidateLegacyScript(script); err != nil {
		return machineError(exitArguments, err)
	}
	var body []byte
	if editor {
		body = []byte("#!/usr/bin/env bash\n\n# " + script.Description + "\n\n")
	} else {
		body, err = readManagedBody(cmd, p)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	if !editor {
		if err := generateManagedMetadata(cmd, managedGeneration{Script: &script, Description: p.generateDescription, Arguments: p.generateArguments, Body: string(body), HasBody: true, Interactive: len(args) == 0, Form: form}); err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	svc := service.New(repository.New(dir))
	if err := svc.Create(cmd.Context(), script, body); err != nil {
		return managedStorageError(err)
	}
	if editor && os.Getenv("SKIP_EDITOR") != "1" {
		if err := svc.OpenEditor(cmd.Context(), script.Name); err != nil {
			return managedStorageError(err)
		}
	}
	if editor && (p.generateDescription || p.generateArguments) {
		if err := generateSavedScriptMetadata(cmd, svc, managedGeneration{Script: &script, Description: p.generateDescription, Arguments: p.generateArguments, Interactive: len(args) == 0, Form: form}); err != nil {
			return managedStorageError(err)
		}
	}
	if p.json {
		saved, err := svc.Find(cmd.Context(), script.Name)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		record := scriptRecord(saved)
		return writeManagedJSON(cmd, managedKindScript, &record, nil)
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyCliScriptQCreatedSuccessfully), script.Name)
}

func generateSavedScriptMetadata(cmd *cobra.Command, svc *service.Service, request managedGeneration) error {
	snapshot, err := svc.Snapshot(cmd.Context(), request.Script.Name)
	if err != nil {
		return fmt.Errorf("snapshot created script: %w", err)
	}
	candidate := snapshot.Script
	request.Script = &candidate
	request.Source = &snapshot.Body
	if err := generateManagedMetadata(cmd, request); err != nil {
		return fmt.Errorf("generate created script metadata: %w", err)
	}
	var patch service.Patch
	if request.Description {
		patch.Description = &candidate.Description
	}
	if request.Arguments {
		patch.Arguments = &candidate.Arguments
	}
	if _, err := svc.Patch(cmd.Context(), snapshot, patch); err != nil {
		return fmt.Errorf("save created script metadata: %w", err)
	}
	return nil
}

//nolint:gocognit // Metadata and body modes are validated before storage and stdin access.
func runScriptEdit(cmd *cobra.Command, args []string) error {
	if _, err := config.GetZevDir(); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	p, err := readManagedFields(cmd, args, managedKindScript, managedOpEdit)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	metadataEdit := p.description != nil || p.arguments != nil || p.generateDescription || p.generateArguments
	bodyEdit := p.bodyFile != "" || p.bodyStdin || p.editBody
	interactive := !metadataEdit && !bodyEdit
	if len(args) == 0 || interactive {
		if err := requireManagedInteractive(cmd, p); err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	if metadataEdit && bodyEdit {
		return usageError(locale.KeyManagedCombinedUnavailable)
	}
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	svc := service.New(repository.New(dir))
	form := newManagedForm(cmd)
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		all, err := svc.List()
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Name
		}
		name, err = form.selectName(names)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	snapshot, err := svc.Snapshot(cmd.Context(), name)
	if err != nil {
		return machineError(exitArguments, err)
	}
	if interactive {
		desc, arguments, err := form.metadata(snapshot.Script.Description, snapshot.Script.Arguments)
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		p.description = &desc
		p.arguments = &arguments
	}
	candidate := snapshot.Script
	if p.description != nil {
		candidate.Description = *p.description
	}
	if p.arguments != nil {
		candidate.Arguments = *p.arguments
	}
	if err := generateManagedMetadata(cmd, managedGeneration{Script: &candidate, Source: &snapshot.Body, Description: p.generateDescription, Arguments: p.generateArguments, Interactive: len(args) == 0 || interactive, Form: form}); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	if p.generateDescription {
		p.description = &candidate.Description
	}
	if p.generateArguments {
		p.arguments = &candidate.Arguments
	}
	if err := metadata.ValidateScript(candidate); err != nil {
		return machineError(exitArguments, err)
	}
	switch {
	case p.editBody:
		err = svc.EditBody(cmd.Context(), snapshot.Body)
	case p.bodyFile != "" || p.bodyStdin:
		var body []byte
		body, err = readManagedBody(cmd, p)
		if err == nil {
			err = svc.ReplaceBody(cmd.Context(), repository.ReplaceParams{Source: snapshot.Body, Body: body})
		}
	default:
		candidate, err = svc.Patch(cmd.Context(), snapshot, service.Patch{Description: p.description, Arguments: p.arguments})
	}
	if err != nil {
		return managedStorageError(err)
	}
	if p.json {
		record := scriptRecord(candidate)
		return writeManagedJSON(cmd, managedKindScript, &record, nil)
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyCliScriptQUpdatedSuccessfully), name)
}

func runScriptRemove(cmd *cobra.Command, args []string) error {
	if _, err := config.GetZevDir(); err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	p, err := readManagedFields(cmd, nil, managedKindScript, managedOpRemove)
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	if len(args) == 0 {
		if err := requireManagedInteractive(cmd, p); err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
	}
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("manage entity: %w", err)
	}
	svc := service.New(repository.New(dir))
	var name string
	//nolint:nestif // An unnamed remove selects a record and separately confirms it.
	if len(args) > 0 {
		name = args[0]
	} else {
		all, err := svc.List()
		if err != nil {
			return fmt.Errorf("manage entity: %w", err)
		}
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Name
		}
		form := newManagedForm(cmd)
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
	removed, err := svc.Find(cmd.Context(), name)
	if err != nil {
		return machineError(exitArguments, err)
	}
	if err := svc.Remove(name); err != nil {
		return managedStorageError(err)
	}
	if p.json {
		record := scriptRecord(removed)
		return writeManagedJSON(cmd, managedKindScript, &record, nil)
	}
	return writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyCliScriptQRemovedSuccessfully), name)
}

func runScriptList(cmd *cobra.Command, _ []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve script store: %w", err)
	}
	scripts, err := service.New(repository.New(dir)).List()
	if err != nil {
		return fmt.Errorf("list scripts: %w", err)
	}
	items := make([]managedRecord, len(scripts))
	for i, script := range scripts {
		items[i] = scriptRecord(script)
		items[i].usage = entityUsage(script.Name, script.Arguments)
	}
	return writeManagedList(cmd, managedKindScript, items)
}

func runScriptShow(cmd *cobra.Command, args []string) error {
	dir, err := config.GetZevDir()
	if err != nil {
		return fmt.Errorf("resolve Zev directory: %w", err)
	}
	if err := service.New(repository.New(dir)).Show(cmd.Context(), args[0], cmd.OutOrStdout()); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliShowScriptError), args[0], err)
	}
	return nil
}
