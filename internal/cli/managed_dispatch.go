package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/metadata"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	scriptservice "github.com/ra1phdd/zev/internal/entities/script/service"
	"github.com/spf13/cobra"
)

// dispatchOriginalArgs only parses the prefix; a matched entity owns its tail.
//
//nolint:gocognit // Original argv lookup must precede query flag parsing and provider setup.
func dispatchOriginalArgs(cmd *cobra.Command, argv []string) (bool, error) {
	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		token := argv[i]
		i++
		if token == "--" {
			break
		}
		switch token {
		case "--option", "--context-file", "-cf", "--tool-approval":
			if i < len(argv) {
				i++
			}
		}
	}
	if i >= len(argv) {
		return false, nil
	}
	// Explicit query/continue commands retain their own query semantics.
	if cmd.Name() == "continue" || cmd.Name() == "query" {
		return false, nil
	}
	if cmd == cmd.Root() && hasRootHelpFlag(argv[:i]) {
		return false, nil
	}
	name := argv[i]
	for _, builtIn := range cmd.Root().Commands() {
		if builtIn.Name() == name {
			return false, nil
		}
	}
	dir, err := config.GetZevDir()
	if err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	aliases, err := aliasrepo.New(dir).LoadAll()
	if err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	alias, found := aliases[name]
	var script domain.Script
	if !found {
		script, err = scriptservice.New(scriptrepo.New(dir)).Find(cmd.Context(), name)
		if errors.Is(err, scriptservice.ErrScriptNotFound) {
			if cmd == cmd.Root() && len(argv[i+1:]) == 1 && (argv[i+1] == entityHelpFlag || argv[i+1] == "-h") {
				return true, machineError(exitArguments, err)
			}
			return false, nil
		}
		if err != nil {
			return true, fmt.Errorf("dispatch entity: %w", err)
		}
	}
	flags := parseArgsFlags(argv[:i])
	flags.Args = argv[i:]
	if err := validateMachineFlags(flags); err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	tail, help, err := entityTail(argv[i+1:])
	if err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	if help {
		if found {
			arguments, e := metadata.EffectiveArguments(alias)
			if e != nil {
				return true, fmt.Errorf("read entity contract: %w", e)
			}
			return true, writeEntityHelp(cmd, managedKindAlias, name, alias.Description, alias.Template, arguments)
		}
		return true, writeEntityHelp(cmd, managedKindScript, name, script.Description, script.ScriptFile, script.Arguments)
	}
	if !found {
		if flags.Print || flags.JSON {
			return true, machineError(exitArguments, fmt.Errorf(locale.Text(locale.KeyCliScriptQCannotPrintedAutomaticallyZevScript), name, name))
		}
		if err := validateEntityArguments(managedKindScript, name, script.Arguments, tail); err != nil {
			return true, fmt.Errorf("dispatch entity: %w", err)
		}
		return true, managedStorageError(scriptservice.New(scriptrepo.New(dir)).Execute(cmd.Context(), name, tail))
	}
	arguments, err := metadata.EffectiveArguments(alias)
	if err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	if err := validateEntityArguments(managedKindAlias, name, arguments, tail); err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	rendered, err := metadata.RenderAlias(alias, tail)
	if err != nil {
		return true, machineError(exitArguments, err)
	}
	if flags.Print || flags.JSON {
		return true, writeMachineResult(cmd, flags, &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: rendered}}})
	}
	return true, managedStorageError(aliasservice.ExecuteRendered(cmd.Context(), rendered))
}

func hasRootHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == entityHelpFlag {
			return true
		}
	}
	return false
}

func entityTail(tail []string) ([]string, bool, error) {
	for i, token := range tail {
		if token == "--" {
			return append(append([]string{}, tail[:i]...), tail[i+1:]...), false, nil
		}
		if token == "-h" || token == entityHelpFlag {
			if len(tail) != 1 {
				return nil, false, machineError(exitArguments, errors.New(locale.Text(locale.KeyManagedHelpAlone)))
			}
			return nil, true, nil
		}
	}
	return tail, false, nil
}

func entityUsage(name string, arguments []domain.Argument) string {
	usage := "zev " + name
	var usageSb129 strings.Builder
	for _, argument := range arguments {
		if argument.Required {
			usageSb129.WriteString(" <" + argument.Name + ">")
		} else {
			usageSb129.WriteString(" [" + argument.Name + "]")
		}
	}
	usage += usageSb129.String()
	return usage
}

func validateEntityArguments(kind, name string, arguments []domain.Argument, argv []string) error {
	for i, argument := range arguments {
		if argument.Required && i >= len(argv) {
			return machineError(exitArguments, fmt.Errorf(locale.Text(locale.KeyManagedMissingArgument), kind, name, i+1, argument.Name, argument.Description, entityUsage(name, arguments), name))
		}
	}
	return nil
}

func writeEntityHelp(cmd *cobra.Command, kind, name, description, source string, arguments []domain.Argument) error {
	if err := writeQueryOutput(cmd.OutOrStdout(), locale.Text(locale.KeyManagedEntityHelp), kind, name, description, source, entityUsage(name, arguments), entityUsage(name, arguments)); err != nil {
		return err
	}
	for i, argument := range arguments {
		if err := writeQueryOutput(cmd.OutOrStdout(), "%d\t%s\t%t\t%s\n", i+1, argument.Name, argument.Required, argument.Description); err != nil {
			return err
		}
	}
	return nil
}
