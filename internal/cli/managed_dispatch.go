package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"github.com/ra1phdd/zev/internal/core/security"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	scriptservice "github.com/ra1phdd/zev/internal/entities/script/service"
	aliasmigration "github.com/ra1phdd/zev/internal/migrations/aliases"
	scriptmigration "github.com/ra1phdd/zev/internal/migrations/scripts"
	"github.com/ra1phdd/zev/pkg/terminal"
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
		case "--option", "--context-file", "-cf", runToolApprovalFlagName:
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
	flags := parseArgsFlags(argv[:i])
	flags.Args = argv[i:]
	// A query may complete a flag combination after its first word. An entity's
	// tail, by contrast, never participates in root flag validation.
	prefixErr := validateMachineFlags(flags)
	if prefixErr != nil && validateMachineFlags(parseArgsFlags(argv)) != nil {
		return true, prefixErr
	}
	tail, help, tailErr := entityTail(argv[i+1:])
	dir, alias, script, err := lookupNamedEntity(cmd.Context(), name, help || tailErr != nil)
	if err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	if alias == nil && script == nil {
		if help {
			return true, machineError(exitArguments, fmt.Errorf("%w: %s", scriptservice.ErrScriptNotFound, name))
		}
		return false, nil
	}
	if prefixErr != nil {
		return true, prefixErr
	}
	if tailErr != nil {
		return true, tailErr
	}
	if help {
		if alias != nil {
			arguments, e := metadata.EffectiveArguments(*alias)
			if e != nil {
				return true, fmt.Errorf("read entity contract: %w", e)
			}
			return true, writeEntityHelp(cmd, managedKindAlias, name, alias.Description, alias.Template, arguments)
		}
		return true, writeEntityHelp(cmd, managedKindScript, name, script.Description, script.ScriptFile, script.Arguments)
	}
	if script != nil {
		if flags.Print || flags.JSON {
			return true, machineError(exitArguments, fmt.Errorf(locale.Text(locale.KeyCliScriptQCannotPrintedAutomaticallyZevScript), name, name))
		}
		if err := validateEntityArguments(managedKindScript, name, script.Arguments, tail); err != nil {
			return true, fmt.Errorf("dispatch entity: %w", err)
		}
		service := scriptservice.New(scriptrepo.New(dir))
		return true, managedStorageError(service.ExecuteAuthorized(cmd.Context(), name, tail, func(current domain.Script, body []byte) error {
			return authorizeManagedScript(cmd, current, body, tail)
		}))
	}
	arguments, err := metadata.EffectiveArguments(*alias)
	if err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	if err := validateEntityArguments(managedKindAlias, name, arguments, tail); err != nil {
		return true, fmt.Errorf("dispatch entity: %w", err)
	}
	rendered, err := metadata.RenderAlias(*alias, tail)
	if err != nil {
		return true, machineError(exitArguments, err)
	}
	if flags.Print || flags.JSON {
		return true, writeMachineResult(cmd, flags, &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: rendered}}})
	}
	if err := authorizeManagedCommand(cmd, rendered); err != nil {
		return true, fmt.Errorf("authorize alias execution: %w", err)
	}
	return true, managedStorageError(aliasservice.ExecuteRendered(cmd.Context(), rendered))
}

func authorizeManagedScript(cmd *cobra.Command, script domain.Script, body []byte, args []string) error {
	cfg, err := loadConfig(cmd.Context())
	if err != nil {
		return fmt.Errorf("load security configuration: %w", err)
	}
	hash := sha256.Sum256(body)
	command := "script " + script.Name
	if len(args) > 0 {
		command += " " + strings.Join(args, " ")
	}
	selected := &domain.Command{
		Command:              command,
		IsDangerous:          true,
		DangerousExplanation: "Saved script snapshot sha256=" + hex.EncodeToString(hash[:]) + " is not fully analyzed and requires confirmation.",
	}
	if err := writeQueryWarning(cmd.ErrOrStderr(), selected); err != nil {
		return err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve script working directory: %w", err)
	}
	policy := cfg.Security.Policy().WithContext(workingDir, homeDirectory())
	action := security.Action{
		Source:        security.SourceScript,
		Provenance:    security.Provenance{Kind: "saved_script", Identifier: script.Name, Digest: hex.EncodeToString(hash[:]), Trusted: true},
		Delivery:      security.DeliveryExecute,
		Dangerous:     true,
		AnalysisKnown: false,
		Certainty:     security.CertaintyUnknown,
		Effects:       []security.Effect{security.EffectProcess, security.EffectFilesystem},
		Interpreter:   "bash",
		WorkingDir:    workingDir,
		Executable:    script.Name,
		Argv:          append([]string{"script", script.Name}, args...),
	}
	if err := (security.CapabilitySet{security.HostActionCapability(action)}).Require(action); err != nil {
		return fmt.Errorf("script capability is not enforced: %w", err)
	}
	decision := policy.Evaluate(action)
	grant, err := security.NewGrant(action, policy, security.SanitizedEnvironment(os.Environ()), body)
	if err != nil {
		return fmt.Errorf("create script security grant: %w", err)
	}
	switch decision {
	case security.DecisionAsk:
		approved, err := confirmQueryExecution(cmd)(cmd.Context(), selected)
		if err != nil {
			return fmt.Errorf("confirm script execution: %w", err)
		}
		if !approved {
			return errors.New("script execution was not approved")
		}
	case security.DecisionAllow:
		// The grant still binds the exact snapshot before dispatch.
	case security.DecisionDeny:
		return errors.New("script execution was denied by security policy")
	}
	if err := grant.Consume(action, policy, security.SanitizedEnvironment(os.Environ()), body); err != nil {
		return fmt.Errorf("validate script security grant: %w", err)
	}
	return nil
}

func authorizeManagedCommand(cmd *cobra.Command, command string) error {
	cfg, err := loadConfig(cmd.Context())
	if err != nil {
		return fmt.Errorf("load security configuration: %w", err)
	}
	assessment := queryservice.ValidateCommandForShell(command, terminal.DefaultShell())
	needsReview := assessment.Dangerous || !assessment.Known
	selected := &domain.Command{
		Command:              command,
		IsDangerous:          needsReview,
		DangerousExplanation: "Saved alias or script execution is not fully analyzed and requires confirmation.",
	}
	if assessment.Dangerous {
		selected.DangerousExplanation = assessment.Explanation
	}
	if needsReview {
		if err := writeQueryWarning(cmd.ErrOrStderr(), selected); err != nil {
			return err
		}
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve alias working directory: %w", err)
	}
	policy := cfg.Security.Policy().WithContext(workingDir, homeDirectory())
	action := security.Action{
		Source:        security.SourceAlias,
		Provenance:    security.Provenance{Kind: "saved_alias", Identifier: command, Trusted: true},
		Delivery:      security.DeliveryExecute,
		Dangerous:     assessment.Dangerous,
		AnalysisKnown: assessment.Known,
		Certainty:     runAssessmentCertainty(assessment.Known),
		Effects:       []security.Effect{security.EffectProcess, security.EffectTerminal},
		Interpreter:   "shell",
		WorkingDir:    workingDir,
		Executable:    command,
	}
	if err := (security.CapabilitySet{security.HostActionCapability(action)}).Require(action); err != nil {
		return fmt.Errorf("alias capability is not enforced: %w", err)
	}
	decision := policy.Evaluate(action)
	grant, err := security.NewGrant(action, policy, security.SanitizedEnvironment(os.Environ()), []byte(command))
	if err != nil {
		return fmt.Errorf("create alias security grant: %w", err)
	}
	switch decision {
	case security.DecisionAllow:
		// The grant still binds the exact snapshot before dispatch.
	case security.DecisionAsk:
		approved, err := confirmQueryExecution(cmd)(cmd.Context(), selected)
		if err != nil {
			return fmt.Errorf("confirm managed command: %w", err)
		}
		if !approved {
			return errors.New("managed command execution was not approved")
		}
	case security.DecisionDeny:
		return errors.New("managed command execution was denied by security policy")
	}
	if err := grant.Consume(action, policy, security.SanitizedEnvironment(os.Environ()), []byte(command)); err != nil {
		return fmt.Errorf("validate alias security grant: %w", err)
	}
	return nil
}

// lookupNamedEntity preserves alias priority. Script storage (including bodies
// required by a legacy conversion) is touched only after an alias miss.
func lookupNamedEntity(ctx context.Context, name string, readOnly bool) (string, *domain.Alias, *domain.Script, error) {
	dir, present, err := zevDirectory()
	if err != nil || !present {
		return dir, nil, nil, err
	}
	var aliases map[string]domain.Alias
	if readOnly {
		aliases, err = readStoreSnapshot(dir, "aliases", metadata.DecodeAliases)
	} else if err = aliasmigration.Migrate(ctx, dir); err == nil {
		aliases, err = aliasrepo.New(dir).LoadAllContext(ctx)
	}
	if err != nil && (!readOnly || !errors.Is(err, os.ErrNotExist)) {
		return dir, nil, nil, fmt.Errorf("read aliases: %w", err)
	}
	if alias, found := aliases[name]; found {
		return dir, &alias, nil, nil
	}
	var scripts map[string]domain.Script
	if readOnly {
		scripts, err = readStoreSnapshot(dir, "scripts", metadata.DecodeScripts)
	} else if err = scriptmigration.Migrate(ctx, dir); err == nil {
		scripts, err = scriptrepo.New(dir).LoadAllContext(ctx)
	}
	if err != nil && (!readOnly || !errors.Is(err, os.ErrNotExist)) {
		return dir, nil, nil, fmt.Errorf("read scripts: %w", err)
	}
	if script, found := scripts[name]; found {
		return dir, nil, &script, nil
	}
	return dir, nil, nil, nil
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
