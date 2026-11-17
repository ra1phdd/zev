package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	"github.com/ra1phdd/zev/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	"github.com/ra1phdd/zev/locale"
	"github.com/spf13/cobra"
)

const (
	exitSuccess         = 0
	exitArguments       = 2
	exitProvider        = 3
	exitInvalidResponse = 4
	exitContext         = 5
)

type machineExitError struct {
	code int
	err  error
}

func (e *machineExitError) Error() string { return e.err.Error() }
func (e *machineExitError) Unwrap() error { return e.err }
func machineError(code int, err error) error {
	if err == nil {
		return nil
	}
	return &machineExitError{code: code, err: err}
}

func validateMachineFlags(flags parsedFlags) error {
	if flags.ParseError != "" {
		return machineError(exitArguments, errors.New(flags.ParseError))
	}
	if err := validateToolFlags(flags); err != nil {
		return machineError(exitArguments, err)
	}
	if flags.Print && flags.JSON {
		return machineError(exitArguments, errors.New(locale.Text(locale.KeyCliPrintJSONCannotUsedTogether)))
	}
	if flags.Option < 1 {
		return machineError(exitArguments, errors.New(locale.Text(locale.KeyCliOptionMustPositiveNumber)))
	}
	if flags.OptionSet && !flags.Print && !flags.JSON {
		return machineError(exitArguments, errors.New(locale.Text(locale.KeyCliOptionRequiresPrintJSON)))
	}
	if (flags.Print || flags.JSON) && (flags.Execute || flags.Inject) {
		return machineError(exitArguments, errors.New(locale.Text(locale.KeyCliMachineOutputCannotCombinedExecuteInject)))
	}
	return nil
}

type machineOutput struct {
	SchemaVersion  int              `json:"schema_version"` //nolint:tagliatelle // Versioned machine CLI contract uses snake_case.
	Status         string           `json:"status"`
	SelectedOption int              `json:"selected_option"` //nolint:tagliatelle // Versioned machine CLI contract uses snake_case.
	Commands       []domain.Command `json:"commands"`
}

func writeMachineResult(cmd *cobra.Command, flags parsedFlags, response *domain.OptionsResponse) error {
	if err := response.Validate(); err != nil {
		return machineError(exitInvalidResponse, err)
	}
	if !response.IsValid {
		return machineError(exitInvalidResponse, fmt.Errorf(locale.Text(locale.KeyCliProviderReturnedNoCommandOptionsMessage), response.ExplanationIfNotValid))
	}
	option := flags.Option
	if option == 0 {
		option = 1
	}
	if option > len(response.Commands) {
		return machineError(exitArguments, fmt.Errorf(locale.Text(locale.KeyCliOptionDNotAvailableProviderReturnedSummary), option, len(response.Commands)))
	}
	selected := response.Commands[option-1]
	if selected.IsDangerous {
		if err := writeQueryWarning(cmd.ErrOrStderr(), &selected); err != nil {
			return err
		}
	}
	if flags.JSON {
		result := machineOutput{SchemaVersion: 1, Status: "ok", SelectedOption: option, Commands: response.Commands}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
			return fmt.Errorf("write machine JSON: %w", err)
		}
	} else if _, err := fmt.Fprintln(cmd.OutOrStdout(), selected.Command); err != nil {
		return fmt.Errorf("write selected command: %w", err)
	}
	return nil
}

func machineProviderError(err error) error {
	if errors.Is(err, domain.ErrInvalidOptionsResponse) || errors.Is(err, domain.ErrNoResponse) {
		return machineError(exitInvalidResponse, err)
	}
	return machineError(exitProvider, err)
}

func machineNamedQuery(cmd *cobra.Command, flags parsedFlags) (bool, error) {
	words := strings.Fields(strings.Join(flags.Args, " "))
	if len(words) == 0 {
		return false, nil
	}
	name, args := words[0], words[1:]
	dir, err := config.GetZevDir()
	if err != nil {
		return true, machineError(exitProvider, err)
	}
	resolved, err := aliasservice.New(aliasrepo.New(dir)).Resolve(name, args)
	if err == nil {
		response := &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: resolved}}}
		queryservice.ApplyDangerousValidation(response)
		return true, writeMachineResult(cmd, flags, response)
	}
	if err != nil && !errors.Is(err, aliasservice.ErrAliasNotFound) {
		return true, fmt.Errorf(locale.Text(locale.KeyCliResolveAliasError), err)
	}
	scripts, err := scriptrepo.New(dir).LoadAllContext(cmd.Context())
	if err != nil {
		return true, fmt.Errorf(locale.Text(locale.KeyCliReadScriptMetadataError), err)
	}
	if _, ok := scripts[name]; ok {
		return true, machineError(exitArguments, fmt.Errorf(locale.Text(locale.KeyCliScriptQCannotPrintedAutomaticallyZevScript), name, name))
	}
	return false, nil
}

func runMachineConfigured(cmd *cobra.Command, flags parsedFlags, logger *slog.Logger) error {
	prompt := strings.Join(flags.Args, " ")
	if strings.TrimSpace(prompt) == "" {
		return machineError(exitArguments, errors.New(locale.Text(locale.KeyCliMachineOutputRequiresNonemptyQuery)))
	}
	projectContext, err := loadProjectContext(flags.ContextFiles, flags.AllowSensitiveContext)
	if err != nil {
		return machineError(exitContext, err)
	}
	if handled, err := machineNamedQuery(cmd, flags); handled {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return machineError(exitProvider, fmt.Errorf(locale.Text(locale.KeyCliLoadProviderConfigurationError), err))
	}
	provider, err := factory.NewProvider(cfg, httpclient.New(cfg.HTTPHeaders, logger))
	if err != nil {
		return machineError(exitProvider, err)
	}
	contextFn := queryservice.New(provider).GetEnvContextChecked
	if !cfg.ContextEnabled || flags.NoContext {
		contextFn = func() (string, error) { return queryLocalContextDisabled, nil }
	}
	return runMachineProvider(cmd, flags, machineProviderParams{Context: contextFn, Provider: provider, Prompt: prompt, ProjectContext: projectContext, History: cfg.HistoryEnabled && !flags.NoHistory, Logger: logger, ToolConfig: cfg})
}

type machineProviderParams struct {
	ToolConfig     *config.Config
	Context        func() (string, error)
	Provider       domain.InferenceProvider
	Prompt         string
	ProjectContext string
	History        bool
	Logger         *slog.Logger
}

func runMachineProvider(cmd *cobra.Command, flags parsedFlags, params machineProviderParams) (resultErr error) {
	env, err := params.Context()
	if err != nil {
		return machineError(exitContext, fmt.Errorf(locale.Text(locale.KeyCliReadLocalContextError), err))
	}
	cfg := params.ToolConfig
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	params.Provider, err = configureQueryTools(cmd, toolProviderParams{Config: cfg, Flags: flags, Provider: params.Provider})
	if err != nil {
		return machineProviderError(err)
	}
	defer func() { resultErr = errors.Join(resultErr, closeQueryTools(params.Provider)) }()
	projectPrompt := env
	if params.ProjectContext != "" {
		projectPrompt += "\n\n" + projectContextSection(params.ProjectContext)
	}
	s := &querySession{
		svc: queryservice.New(params.Provider), cmd: cmd, userPrompt: params.Prompt,
		history: &domain.ConversationHistory{SystemPrompt: fmt.Sprintf(repository.SystemPrompt, projectPrompt), Messages: []domain.Message{{Role: queryUserRole, Content: params.Prompt}}}, machine: true,
	}
	s.provider, s.model = params.Provider.Name(), params.Provider.Model()
	s.prepareHistory(params.History)
	defer s.closeHistory(cmd.Context(), &resultErr)
	if s.persistence != nil {
		s.warnHistory(s.persistence.Start(cmd.Context(), params.Prompt, s.provider, s.model))
	}
	if err := s.fetch(cmd.Context()); err != nil {
		return machineProviderError(err)
	}
	if _, err := s.acceptResponse(); err != nil {
		return err
	}
	if err := writeMachineResult(cmd, flags, s.response); err != nil {
		return err
	}
	if s.persistence != nil {
		s.warnHistory(s.persistence.Select(cmd.Context(), historyservice.SelectionOutcome{Index: flags.Option - 1, DeliveryOutcome: queryDeliveryPrinted}))
	}
	return nil
}
