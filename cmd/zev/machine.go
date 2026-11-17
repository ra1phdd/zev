package main

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

func cliExitCode(err error) int {
	if err == nil {
		return exitSuccess
	}
	if classified, ok := errors.AsType[*machineExitError](err); ok {
		return classified.code
	}
	return 1
}

func validateMachineFlags(flags parsedFlags) error {
	if flags.ParseError != "" {
		return machineError(exitArguments, errors.New(flags.ParseError))
	}
	if flags.Print && flags.JSON {
		return machineError(exitArguments, errors.New("--print and --json cannot be used together"))
	}
	if flags.Option < 1 {
		return machineError(exitArguments, errors.New("--option must be a positive number"))
	}
	if flags.OptionSet && !flags.Print && !flags.JSON {
		return machineError(exitArguments, errors.New("--option requires --print or --json"))
	}
	if (flags.Print || flags.JSON) && (flags.Execute || flags.Inject) {
		return machineError(exitArguments, errors.New("machine output cannot be combined with --execute or --inject"))
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
		return machineError(exitInvalidResponse, fmt.Errorf("provider returned no command options: %s", response.ExplanationIfNotValid))
	}
	option := flags.Option
	if option == 0 {
		option = 1
	}
	if option > len(response.Commands) {
		return machineError(exitArguments, fmt.Errorf("option %d is not available; provider returned %d option(s)", option, len(response.Commands)))
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
	resolved, err := aliasservice.New(aliasrepo.New(config.GetZevDir())).Resolve(name, args)
	if err == nil {
		response := &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: resolved}}}
		queryservice.ApplyDangerousValidation(response)
		return true, writeMachineResult(cmd, flags, response)
	}
	if err != nil && !errors.Is(err, aliasservice.ErrAliasNotFound) {
		return true, fmt.Errorf("resolve alias: %w", err)
	}
	scripts, err := scriptrepo.New(config.GetZevDir()).LoadAllContext(cmd.Context())
	if err != nil {
		return true, fmt.Errorf("read script metadata: %w", err)
	}
	if _, ok := scripts[name]; ok {
		return true, machineError(exitArguments, fmt.Errorf("script %q cannot be printed automatically; use `zev script show %s`", name, name))
	}
	return false, nil
}

func runMachineConfigured(cmd *cobra.Command, flags parsedFlags, logger *slog.Logger) error {
	prompt := strings.Join(flags.Args, " ")
	if strings.TrimSpace(prompt) == "" {
		return machineError(exitArguments, errors.New("machine output requires a nonempty query"))
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
		return machineError(exitProvider, fmt.Errorf("load provider configuration: %w", err))
	}
	provider, err := factory.NewProvider(cfg, httpclient.New(cfg.HTTPHeaders, logger))
	if err != nil {
		return machineError(exitProvider, err)
	}
	contextFn := queryservice.New(provider).GetEnvContextChecked
	if !cfg.ContextEnabled || flags.NoContext {
		contextFn = func() (string, error) { return "Local environment context is disabled.", nil }
	}
	return runMachineProvider(cmd, flags, machineProviderParams{Context: contextFn, Provider: provider, Prompt: prompt, ProjectContext: projectContext, History: cfg.HistoryEnabled && !flags.NoHistory, Logger: logger})
}

type machineProviderParams struct {
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
		return machineError(exitContext, fmt.Errorf("read local context: %w", err))
	}
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
		s.warnHistory(s.persistence.Select(cmd.Context(), historyservice.SelectionOutcome{Index: flags.Option - 1, DeliveryOutcome: "printed"}))
	}
	return nil
}
