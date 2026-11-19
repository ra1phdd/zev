package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/core/security"
	historyrepo "github.com/ra1phdd/zev/internal/entities/history/repository"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	"github.com/ra1phdd/zev/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	"github.com/ra1phdd/zev/pkg/terminal"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
	"github.com/spf13/cobra"
)

const (
	queryCancelled            = "cancelled"
	queryUserRole             = "user"
	queryAssistantRole        = "assistant"
	queryRefineMarker         = "::REFINE_QUERY::"
	continueQueryFlag         = "--continue"
	queryLocalContextDisabled = "Local environment context is disabled."
	queryDeliverySelected     = "selected"
	queryDeliveryPrinted      = "printed"
	queryDeliveryExecuted     = "executed"
	queryDeliveryUnknown      = "unknown"
)

var (
	executeFlag       bool
	injectFlag        bool
	noHistoryFlag     bool
	noContextFlag     bool
	errQueryCancelled = errors.New(queryCancelled)
)

// parsedFlags contains manually parsed CLI flags without changing package state.
type parsedFlags struct {
	Args                  []string
	Help                  bool
	Verbosity             int
	Execute               bool
	Inject                bool
	NoHistory             bool
	NoContext             bool
	Continue              bool
	Proxy                 string
	AllowSensitiveContext bool
	ContextFiles          []string
	Print                 bool
	JSON                  bool
	Option                int
	OptionSet             bool
	Tools                 bool
	NoTools               bool
	ToolApproval          string
	ToolApprovalSet       bool
	ParseError            string
}

// setupLogger enables warnings by default, info at -v and debug at -vv.
// At -vvv and above it also includes source locations.
func setupLogger(verbosity int) *slog.Logger {
	var level slog.Level
	var addSource bool
	switch verbosity {
	case 0:
		level = slog.LevelWarn
	case 1:
		level = slog.LevelInfo
	case 2:
		level = slog.LevelDebug
	default:
		level = slog.LevelDebug
		addSource = true
	}
	opts := &slog.HandlerOptions{Level: level, AddSource: addSource}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

func newContinueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "continue [clarification]",
		Short:              locale.Text(locale.KeyQueryContinueLatestUsableConversation),
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(cmd, append([]string{"--continue"}, args...))
		},
	}
	registerToolFlags(cmd)
	return cmd
}

func newQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query [query]",
		Short: locale.Text(locale.KeyQueryQueryCommandSuggestions),
		Args:  cobra.ArbitraryArgs,
		RunE:  runQuery,
	}
	cmd.Flags().BoolVar(&executeFlag, "execute", false, locale.Text(locale.KeyQueryExecuteSelectedCommandImmediately))
	cmd.Flags().BoolVar(&injectFlag, "inject", false, locale.Text(locale.KeyQueryInjectSelectedCommandIntoTerminalDefault))
	cmd.Flags().BoolVar(&noHistoryFlag, "no-history", false, locale.Text(locale.KeyQueryNotPersistQueryHistory))
	cmd.Flags().BoolVar(&noContextFlag, "no-context", false, locale.Text(locale.KeyQueryNotSendLocalEnvironmentContext))
	registerToolFlags(cmd)
	cmd.DisableFlagParsing = true
	cmd.Flags().Bool("print", false, locale.Text(locale.KeyQueryPrintChosenCommandExecuting))
	cmd.Flags().Bool("json", false, locale.Text(locale.KeyQueryOutputCommandOptionsSchemaVersion1JSON))
	cmd.Flags().Int("option", 1, locale.Text(locale.KeyQueryChoose1BasedCommandOptionMachineMode))
	cmd.Flags().StringArray("context-file", nil, locale.Text(locale.KeyCliAttachProjectContextFileRepeatableShortForm))
	cmd.Flags().Bool("allow-sensitive-context", false, locale.Text(locale.KeyCliAllowExplicitlyRequestedSensitiveContextPathsSymlink))
	cmd.MarkFlagsMutuallyExclusive("execute", "inject")
	return cmd
}

// resolveExecutionMode gives flags priority over configuration and defaults to inject.
func resolveExecutionMode(cfg *config.Config, executeFlag, injectFlag bool) string {
	if executeFlag {
		return executionModeExecute
	}
	if injectFlag {
		return executionModeInject
	}
	if cfg.CommandExecutionMode != "" {
		return cfg.CommandExecutionMode
	}
	return executionModeInject
}

// parseArgsFlags preserves unknown flags for aliases while the root command uses
// DisableFlagParsing. It does not mutate package state.
//
//nolint:gocognit // Manual flag parsing must preserve unknown alias arguments and -- separators.
func parseArgsFlags(args []string) parsedFlags {
	result := parsedFlags{Args: []string{}, Option: 1}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			result.Args = append(result.Args, args[i+1:]...)
			break
		}
		switch {
		case arg == "-h", arg == entityHelpFlag:
			result.Help = true
		case arg == "-v", arg == "--verbose":
			result.Verbosity++
		case strings.HasPrefix(arg, "--verbose="):
			if n, err := strconv.Atoi(strings.TrimPrefix(arg, "--verbose=")); err == nil && n > 0 {
				result.Verbosity = n
			}
		case len(arg) > 2 && strings.HasPrefix(arg, "-vv"):
			result.Verbosity = len(arg) - 1
		case arg == "-e", arg == "--execute":
			result.Execute = true
		case arg == "--inject":
			result.Inject = true
		case arg == "--no-history":
			result.NoHistory = true
		case arg == "--no-context":
			result.NoContext = true
		case arg == "--tools":
			result.Tools = true
		case arg == "--no-tools":
			result.NoTools = true
		case arg == runToolApprovalFlagName:
			result.ToolApprovalSet = true
			if i+1 >= len(args) {
				result.ParseError = "--tool-approval requires a mode"
				continue
			}
			i++
			result.ToolApproval = args[i]
		case strings.HasPrefix(arg, "--tool-approval="):
			result.ToolApprovalSet = true
			result.ToolApproval = strings.TrimPrefix(arg, "--tool-approval=")
		case arg == "--continue":
			result.Continue = true
		case arg == cliProxyFlag:
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
				result.ParseError = "--proxy requires a URL"
				continue
			}
			i++
			result.Proxy = args[i]
		case strings.HasPrefix(arg, "--proxy="):
			result.Proxy = strings.TrimPrefix(arg, "--proxy=")
			if result.Proxy == "" {
				result.ParseError = "--proxy requires a URL"
			}
		case arg == "--allow-sensitive-context":
			result.AllowSensitiveContext = true
		case arg == "--context-file", arg == "-cf":
			if i+1 >= len(args) {
				result.ParseError = arg + " requires a path"
				continue
			}
			i++
			result.ContextFiles = append(result.ContextFiles, args[i])
		case strings.HasPrefix(arg, "--context-file="):
			path := strings.TrimPrefix(arg, "--context-file=")
			if path == "" {
				result.ParseError = "--context-file requires a path"
			} else {
				result.ContextFiles = append(result.ContextFiles, path)
			}
		case arg == "--print":
			result.Print = true
		case arg == "--json":
			result.JSON = true
		case arg == "--option":
			result.OptionSet = true
			if i+1 >= len(args) {
				result.ParseError = "--option requires a value"
				continue
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil {
				result.ParseError = "--option must be a number"
			} else {
				result.Option = value
			}
		case strings.HasPrefix(arg, "--option="):
			result.OptionSet = true
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--option="))
			if err != nil {
				result.ParseError = "--option must be a number"
			} else {
				result.Option = value
			}
		default:
			result.Args = append(result.Args, arg)
		}
	}
	return result
}

func runQuery(cmd *cobra.Command, args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		if err := cmd.Help(); err != nil {
			return fmt.Errorf("show query help: %w", err)
		}
		return nil
	}
	if handled, err := dispatchOriginalArgs(cmd, args); handled {
		return err
	}
	parsed := parseArgsFlags(args)
	configured, err := config.LoadUILanguage()
	if err != nil {
		configured = toolModeAuto
	}
	// Natural-language queries must be localized from the request itself. A
	// persisted language remains useful for help, but must not pin query UI to
	// English when the request is Russian (or vice versa).
	language := queryUILanguage(configured, parsed.Help)
	defer locale.SetLanguage(language, strings.Join(parsed.Args, " "))()
	parsed.NoHistory = parsed.NoHistory || noHistoryFlag
	parsed.NoContext = parsed.NoContext || noContextFlag
	if parsed.Help {
		if err := cmd.Help(); err != nil {
			return fmt.Errorf("show query help: %w", err)
		}
		return nil
	}
	// Cobra consumes subcommand flags; the root preserves them for manual parsing.
	if cmd.Flags().Changed("execute") {
		parsed.Execute = executeFlag
	}
	if cmd.Flags().Changed("inject") {
		parsed.Inject = injectFlag
	}
	if err := validateMachineFlags(parsed); err != nil {
		return err
	}
	verbosityLevel, executeFlag, injectFlag = parsed.Verbosity, parsed.Execute, parsed.Inject
	logger := setupLogger(verbosityLevel)
	if parsed.Print || parsed.JSON {
		logger = slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelWarn}))
		if parsed.Continue {
			return runContinuationQuery(cmd, parsed, logger)
		}
		return runMachineConfigured(cmd, parsed, logger)
	}
	err = runConfiguredQuery(cmd, parsed, logger)
	if errors.Is(err, errQueryCancelled) {
		return nil
	}
	return err
}

func queryUILanguage(configured string, help bool) string {
	if help {
		return configured
	}
	return "auto"
}

func runConfiguredQuery(cmd *cobra.Command, flags parsedFlags, logger *slog.Logger) (resultErr error) {
	if flags.Continue {
		return runContinuationQuery(cmd, flags, logger)
	}
	args := flags.Args
	projectContext, err := loadProjectContext(flags.ContextFiles, flags.AllowSensitiveContext)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(cmd.Context())
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyQueryFailedLoadConfigWRunZevSetup), err)
	}
	cfg.HistoryEnabled = cfg.HistoryEnabled && !flags.NoHistory
	cfg.ContextEnabled = cfg.ContextEnabled && !flags.NoContext
	logger.Info("Configuration loaded", "provider", cfg.LLMProvider)
	userPrompt := strings.Join(args, " ")
	if len(args) == 0 {
		var err error
		userPrompt, err = readQueryInput(func(title string) (string, error) {
			return prompt.InputContext(cmd.Context(), title)
		}, locale.Text(locale.KeyQueryWhatCommandYouNeed))
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(userPrompt) == "" {
		return errors.New(locale.Text(locale.KeyQueryPromptCannotEmpty))
	}
	locale.UpdateRequestLanguage(userPrompt)
	return runConfiguredProvider(cmd, flags, logger, cfg, userPrompt, projectContext)
}

func runConfiguredProvider(cmd *cobra.Command, flags parsedFlags, logger *slog.Logger, cfg *config.Config, userPrompt, projectContext string) (resultErr error) {
	httpClient, err := httpclient.NewWithProxy(cfg.EffectiveHTTPHeaders(), flags.Proxy, logger)
	if err != nil {
		return fmt.Errorf("configure HTTP proxy: %w", err)
	}
	provider, err := factory.NewProvider(cfg, httpClient)
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyQueryFailedCreateProviderError), err)
	}
	provider, err = configureQueryTools(cmd, toolProviderParams{Config: cfg, Flags: flags, Provider: provider})
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeQueryTools(provider)) }()
	logger.Debug("Provider created", "name", provider.Name(), "model", provider.Model())
	s := newQuerySessionWithProjectContext(queryservice.New(provider), cmd, userPrompt, projectContext)
	s.provider, s.model = provider.Name(), provider.Model()
	s.updateLanguage = locale.UpdateRequestLanguage
	s.prepareHistory(cfg.HistoryEnabled)
	defer s.closeHistory(cmd.Context(), &resultErr)
	if !cfg.ContextEnabled {
		s.history.SystemPrompt = fmt.Sprintf(repository.SystemPrompt, queryLocalContextDisabled)
	}
	if err := s.start(cmd.Context()); err != nil {
		return err
	}
	ready, err := s.acceptResponse()
	if err != nil || !ready {
		return err
	}
	mode := resolveExecutionMode(cfg, executeFlag, injectFlag)
	selected, index, err := s.selectCommand(cmd.Context(), mode)
	if err != nil || selected == nil {
		return err
	}
	outcome := historyservice.SelectionOutcome{Index: index, DeliveryOutcome: queryDeliverySelected}
	if s.persistence != nil {
		s.warnHistory(s.persistence.Select(cmd.Context(), outcome))
	}
	sessionID := ""
	if s.persistence != nil {
		conversationID, commandID, _ := s.persistence.SelectedCommandReference(index)
		if cfg.CommandWrapperEnabled && conversationID != "" && commandID != "" {
			sessionID, err = registerRunSession(cmd.Context(), conversationID, commandID)
			if err != nil {
				s.warnHistory(err)
				sessionID = ""
			}
		}
	}
	err = deliverQueryCommand(cmd.Context(), queryDelivery{
		command: selected, mode: mode, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), logger: logger,
		wrapInject: cfg.CommandWrapperEnabled, sessionID: sessionID,
		securityMode: security.Mode(cfg.Security.Mode), securityDefaultDecision: security.Decision(cfg.Security.DefaultDecision), securityPolicy: cfg.Security.Policy(),
		runner:  &terminal.Runner{Stdin: cmd.InOrStdin(), Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()},
		confirm: confirmQueryExecution(cmd), outcome: &outcome,
		reviewCommand: s.svc.ReviewCommandSafety, userRequest: s.userPrompt,
	})
	if s.persistence != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 5*time.Second)
		s.warnHistory(s.persistence.Select(ctx, outcome))
		cancel()
	}
	return err
}

func openQueryHistory(ctx context.Context) (*historyrepo.Store, error) {
	dir, err := config.GetZevDir()
	if err != nil {
		return nil, fmt.Errorf("resolve Zev directory: %w", err)
	}
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, fmt.Errorf("validate history directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	store, err := openHistoryStore(ctx, filepath.Join(dir, "history.db"))
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	return store, nil
}

// querySession owns only the active conversation and its interactive UI seams.
// Provider metadata and command execution stay in the composition root.
type querySession struct {
	machine              bool
	persistence          *historyservice.Lifecycle
	historyStore         *historyrepo.Store
	provider, model      string
	svc                  *queryservice.Service
	cmd                  *cobra.Command
	history              *domain.ConversationHistory
	startHistory         []domain.Message
	continuationMessages []domain.Message
	userPrompt           string
	projectContext       string
	response             *domain.OptionsResponse
	input                func(string) (string, error)
	selectItem           func(list.SelectOptions) (list.Item, error)
	updateLanguage       func(string)
}

func newQuerySessionWithProjectContext(svc *queryservice.Service, cmd *cobra.Command, userPrompt, projectContext string) *querySession {
	messages := []domain.Message{{Role: queryUserRole, Content: userPrompt}}
	if projectContext != "" {
		messages = []domain.Message{projectContextMessage(projectContext), {Role: queryUserRole, Content: userPrompt}}
	}
	return &querySession{
		svc: svc, cmd: cmd, userPrompt: userPrompt, projectContext: projectContext,
		history: &domain.ConversationHistory{
			SystemPrompt: fmt.Sprintf(repository.SystemPrompt, svc.GetEnvContext()),
			Messages:     messages,
		},
		input: func(title string) (string, error) {
			return prompt.InputContext(cmd.Context(), title)
		},
		selectItem: func(options list.SelectOptions) (list.Item, error) {
			return list.SelectContext(cmd.Context(), options)
		},
	}
}

func newQuerySession(svc *queryservice.Service, cmd *cobra.Command, userPrompt string) *querySession {
	return newQuerySessionWithProjectContext(svc, cmd, userPrompt, "")
}

func readQueryInput(input func(string) (string, error), title string) (string, error) {
	value, err := input(title)
	if err != nil {
		if err.Error() == queryCancelled {
			return "", errQueryCancelled
		}
		return "", fmt.Errorf(locale.Text(locale.KeyQueryGetQueryInputError), err)
	}
	return value, nil
}

func (s *querySession) prepareHistory(enabled bool) {
	if !enabled {
		return
	}
	store, err := openQueryHistory(s.cmd.Context())
	if err != nil {
		s.warnHistory(err)
		return
	}
	s.persistence = historyservice.NewLifecycle(store)
	s.historyStore = store
}

func (s *querySession) closeHistory(ctx context.Context, resultErr *error) {
	if s.historyStore == nil {
		return
	}
	s.finishHistory(ctx, s.historyStore, resultErr)
}

func (s *querySession) finishHistory(ctx context.Context, store *historyrepo.Store, resultErr *error) {
	status := "completed"
	if errors.Is(*resultErr, errQueryCancelled) || errors.Is(*resultErr, context.Canceled) {
		status = "cancelled"
	} else if *resultErr != nil {
		status = historyStatusFailed
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	s.warnHistory(s.persistence.Complete(cleanupCtx, status))
	s.warnHistory(store.Close())
}

func (s *querySession) warnHistory(err error) {
	if err != nil {
		if _, writeErr := fmt.Fprintf(s.cmd.ErrOrStderr(), locale.Text(locale.KeyQueryWarningFailedSaveHistoryMessage), err); writeErr != nil {
			return
		}
	}
}

func (s *querySession) fetch(ctx context.Context) error {
	progressOut := s.cmd.OutOrStdout()
	if s.machine {
		progressOut = s.cmd.ErrOrStderr()
	}
	if err := writeQueryOutput(progressOut, "%s", locale.Text(locale.KeyQueryFetchingSuggestions)); err != nil {
		return err
	}
	response, err := s.svc.GetOptionsWithHistory(ctx, s.history)
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyQueryGetCommandSuggestionsError), err)
	}
	s.response = response
	if s.persistence != nil {
		s.warnHistory(s.persistence.Response(ctx, response))
	}
	return nil
}

func (s *querySession) start(ctx context.Context) error {
	if s.persistence != nil {
		if len(s.startHistory) > 0 {
			prompt := s.userPrompt
			if len(s.continuationMessages) > 0 {
				prompt = ""
			}
			s.warnHistory(s.persistence.StartWithHistory(ctx, historyservice.StartHistoryInput{Prompt: prompt, Provider: s.provider, Model: s.model, ConversationHistory: s.startHistory}))
			for _, message := range s.continuationMessages {
				s.warnHistory(s.persistence.Append(ctx, message.Content, message.Kind))
			}
		} else {
			s.warnHistory(s.persistence.Start(ctx, s.userPrompt, s.provider, s.model))
		}
	}
	err := s.fetch(ctx)
	if !errors.Is(err, domain.ErrNoResponse) {
		return err
	}
	if err := writeQueryOutput(s.cmd.ErrOrStderr(), "%s", locale.Text(locale.KeyQueryAiDidNotProvideResponseMightHappen)); err != nil {
		return err
	}
	refined, err := readQueryInput(s.input, locale.Text(locale.KeyQueryPleaseClarifyRequest))
	if err != nil {
		return err
	}
	if strings.TrimSpace(refined) == "" {
		return errors.New("the request cannot be empty")
	}
	s.addRefinement(refined)
	if s.persistence != nil {
		s.warnHistory(s.persistence.Refine(ctx, refined))
	}
	return s.fetch(ctx)
}

func (s *querySession) addRefinement(value string) {
	if s.updateLanguage != nil {
		s.updateLanguage(value)
	}
	s.userPrompt = value
	s.history.Messages = append(s.history.Messages, domain.Message{Role: queryUserRole, Content: value})
}

func (s *querySession) acceptResponse() (bool, error) {
	security.ApplyDangerousValidation(s.response)
	if err := s.response.Validate(); err != nil {
		if s.machine {
			return false, machineError(4, err)
		}
		return false, fmt.Errorf("validate response before selection: %w", err)
	}
	if !s.response.IsValid {
		if s.machine {
			return false, machineError(4, fmt.Errorf(locale.Text(locale.KeyQueryProviderReturnedInvalidResponseMessage), s.response.ExplanationIfNotValid))
		}
		return false, writeQueryOutput(s.cmd.ErrOrStderr(), locale.Text(locale.KeyQueryUnableProcessRequestMessage), s.response.ExplanationIfNotValid)
	}
	if len(s.response.Commands) == 0 {
		if s.machine {
			return false, machineError(4, errors.New(locale.Text(locale.KeyQueryProviderReturnedNoCommands)))
		}
		return false, writeQueryOutput(s.cmd.OutOrStdout(), "%s", locale.Text(locale.KeyQueryNoCommandsFoundQuery))
	}
	s.history.Messages = append(s.history.Messages, domain.Message{Role: queryAssistantRole, Content: s.response.CompactJSON()})
	return true, nil
}

func (s *querySession) commandItems() []list.Item {
	items := make([]list.Item, 0, len(s.response.Commands)+1)
	for i := range s.response.Commands {
		items = append(items, list.NewCommandItem(&s.response.Commands[i]))
	}
	items = append(items, list.NewCommandItem(&domain.Command{
		Command: queryRefineMarker, ShortExplanation: locale.Text(locale.KeyQueryClarifyRequest),
	}))
	return items
}

func (s *querySession) selectCommand(ctx context.Context, mode string) (*domain.Command, int, error) {
	if mode == executionModeExecute {
		return &s.response.Commands[0], 0, nil
	}
	for {
		selected, err := s.chooseCommand()
		if err != nil {
			return nil, 0, err
		}
		if selected.Command != queryRefineMarker {
			for i := range s.response.Commands {
				if s.response.Commands[i].Command == selected.Command {
					return &s.response.Commands[i], i, nil
				}
			}
			return nil, 0, errors.New("selected command is not in the response")
		}
		ready, err := s.refine(ctx)
		if err != nil || !ready {
			return nil, 0, err
		}
	}
}

func (s *querySession) chooseCommand() (*domain.Command, error) {
	selected, err := s.selectItem(list.SelectOptions{
		Title: locale.Text(locale.KeyQuerySelectCommandRun), Items: s.commandItems(), ShowWarning: true,
	})
	if err != nil {
		if err.Error() == queryCancelled {
			return nil, errQueryCancelled
		}
		return nil, fmt.Errorf(locale.Text(locale.KeyQueryFailedSelectCommandError), err)
	}
	item, ok := selected.(list.CommandItem)
	if !ok || item.Cmd == nil {
		return nil, fmt.Errorf("unexpected command selection %T", selected)
	}
	return item.Cmd, nil
}

func (s *querySession) refine(ctx context.Context) (bool, error) {
	refined, err := readQueryInput(s.input, locale.Text(locale.KeyQuerySpecifyRequest))
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(refined) == "" {
		return true, writeQueryOutput(s.cmd.ErrOrStderr(), "%s", locale.Text(locale.KeyQueryRequestCannotEmpty))
	}
	s.addRefinement(refined)
	if s.persistence != nil {
		s.warnHistory(s.persistence.Refine(ctx, refined))
	}
	err = s.fetch(ctx)
	if errors.Is(err, domain.ErrNoResponse) {
		return true, writeQueryOutput(s.cmd.ErrOrStderr(), "%s", locale.Text(locale.KeyQueryAiDidNotProvideResponsePleaseTry))
	}
	if err != nil {
		return false, err
	}
	return s.acceptResponse()
}

type queryDelivery struct {
	command                 *domain.Command
	wrapInject              bool
	sessionID               string
	userRequest             string
	reviewCommand           func(context.Context, string, string, string) (queryservice.CommandSafetyReview, error)
	mode                    string
	securityMode            security.Mode
	securityDefaultDecision security.Decision
	securityPolicy          security.Policy
	out                     io.Writer
	errOut                  io.Writer
	logger                  *slog.Logger
	runner                  *terminal.Runner
	inject                  func(string) error
	confirm                 func(context.Context, *domain.Command) (bool, error)
	outcome                 *historyservice.SelectionOutcome
}

func deliverQueryCommand(ctx context.Context, delivery queryDelivery) error {
	outcome := delivery.outcome
	if outcome == nil {
		outcome = &historyservice.SelectionOutcome{}
	}
	outcome.DeliveryOutcome = queryDeliverySelected
	selected := delivery.command
	if selected == nil {
		return fmt.Errorf("%w: selected command is nil", domain.ErrInvalidOptionsResponse)
	}
	response := &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{*selected}}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("validate selected command: %w", err)
	}
	if err := ctx.Err(); err != nil {
		outcome.DeliveryOutcome = queryDeliveryUnknown
		outcome.ExecutionError = err.Error()
		return fmt.Errorf("deliver selected command: %w", err)
	}
	if delivery.mode == executionModeExecute {
		return deliverExecuteCommand(ctx, delivery, selected, outcome)
	}
	return deliverInjectCommand(delivery, selected, outcome)
}

func deliverExecuteCommand(ctx context.Context, delivery queryDelivery, selected *domain.Command, outcome *historyservice.SelectionOutcome) error {
	localAssessment := security.ValidateCommandForShell(selected.Command, terminal.DefaultShell())
	localDangerous := localAssessment.Dangerous
	reviewReason := ""
	if !localAssessment.Known {
		review, err := reviewUnknownQueryCommand(ctx, delivery, selected)
		if err != nil {
			return recordDeliveryFailure(outcome, err)
		}
		reviewReason = review.Reason
	}
	if localAssessment.Dangerous {
		selected.IsDangerous = true
		selected.DangerousExplanation = localAssessment.Explanation
	}
	if !localAssessment.Known && selected.DangerousExplanation == "" {
		selected.DangerousExplanation = "Command safety checked by the configured provider: " + reviewReason
	}
	runner, err := queryExecutionRunner(delivery.runner)
	if err != nil {
		return recordDeliveryFailure(outcome, err)
	}
	policy, err := queryExecutionPolicy(delivery, runner)
	if err != nil {
		return recordDeliveryFailure(outcome, err)
	}
	action := security.Action{
		Source:        security.SourceQuery,
		Provenance:    security.Provenance{Kind: "provider_suggestion", Trusted: false},
		Delivery:      security.DeliveryExecute,
		Dangerous:     localDangerous,
		AnalysisKnown: localAssessment.Known || reviewReason != "",
		Certainty:     querySecurityCertainty(localAssessment.Known || reviewReason != ""),
		Effects:       []security.Effect{security.EffectProcess, security.EffectTerminal},
		Interpreter:   terminal.DefaultShell(),
		WorkingDir:    runner.Dir,
	}
	if err := (security.CapabilitySet{security.HostActionCapability(action)}).Require(action); err != nil {
		return recordDeliveryFailure(outcome, fmt.Errorf("query capability is not enforced: %w", err))
	}
	decision := policy.Evaluate(action)
	grant, err := security.NewGrant(action, policy, queryRunnerEnvironment(*runner), []byte(selected.Command))
	if err != nil {
		return recordDeliveryFailure(outcome, fmt.Errorf("create query security grant: %w", err))
	}
	if err := writeQueryWarning(delivery.errOut, selected); err != nil {
		return recordDeliveryFailure(outcome, err)
	}
	if err := authorizeQueryExecution(ctx, delivery, selected, decision); err != nil {
		return recordDeliveryFailure(outcome, err)
	}
	if err := grant.Consume(action, policy, queryRunnerEnvironment(*runner), []byte(selected.Command)); err != nil {
		return recordDeliveryFailure(outcome, fmt.Errorf("validate query security grant: %w", err))
	}
	if err := runner.RunArgv(ctx, terminal.DefaultShell(), []string{"-c", selected.Command}); err != nil {
		outcome.DeliveryOutcome = queryDeliveryUnknown
		outcome.ExecutionError = err.Error()
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			code := exitErr.ExitCode()
			outcome.DeliveryOutcome = historyStatusFailed
			outcome.ExecutionExitCode = &code
		}
		return fmt.Errorf(locale.Text(locale.KeyQueryFailedExecuteCommandError), err)
	}
	now := time.Now()
	code := 0
	outcome.DeliveryOutcome = queryDeliveryExecuted
	outcome.ExecutedAt = &now
	outcome.ExecutionExitCode = &code
	return nil
}

func reviewUnknownQueryCommand(ctx context.Context, delivery queryDelivery, selected *domain.Command) (queryservice.CommandSafetyReview, error) {
	if delivery.reviewCommand == nil || strings.TrimSpace(delivery.userRequest) == "" || strings.TrimSpace(selected.ShortExplanation) == "" {
		return queryservice.CommandSafetyReview{}, errors.New("automatic execution stopped: command could not be checked")
	}
	review, err := delivery.reviewCommand(ctx, selected.Command, delivery.userRequest, selected.ShortExplanation)
	if err != nil {
		return queryservice.CommandSafetyReview{}, fmt.Errorf("automatic execution stopped: command safety review failed: %w", err)
	}
	if !review.Safe {
		return queryservice.CommandSafetyReview{}, fmt.Errorf("automatic execution stopped: provider did not approve the command: %s", review.Reason)
	}
	if strings.TrimSpace(review.Reason) == "" {
		return queryservice.CommandSafetyReview{}, errors.New("automatic execution stopped: provider review was unclear")
	}
	return review, nil
}

func queryRunnerEnvironment(runner terminal.Runner) []string {
	if runner.Env == nil {
		return security.SanitizedEnvironment(os.Environ())
	}
	return append([]string(nil), runner.Env...)
}

func queryExecutionRunner(runner *terminal.Runner) (*terminal.Runner, error) {
	if runner == nil {
		defaultRunner := terminal.DefaultRunner()
		runner = &defaultRunner
	}
	if runner.Dir != "" {
		return runner, nil
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve command working directory: %w", err)
	}
	runner.Dir = workingDir
	return runner, nil
}

func queryExecutionPolicy(delivery queryDelivery, runner *terminal.Runner) (security.Policy, error) {
	policy := delivery.securityPolicy
	if policy.Mode == "" {
		policy = security.Policy{Mode: delivery.securityMode, DefaultDecision: delivery.securityDefaultDecision}
	}
	if policy.Workspace != "" {
		return policy, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return security.Policy{}, fmt.Errorf("resolve policy home: %w", err)
	}
	return policy.WithContext(runner.Dir, home), nil
}

func querySecurityCertainty(known bool) security.Certainty {
	if known {
		return security.CertaintyComplete
	}
	return security.CertaintyUnknown
}

func authorizeQueryExecution(ctx context.Context, delivery queryDelivery, selected *domain.Command, decision security.Decision) error {
	if decision != security.DecisionAsk {
		if decision != security.DecisionAllow {
			return errors.New("command execution was denied by security policy")
		}
		return nil
	}
	if delivery.confirm == nil {
		return errors.New("execute requires interactive confirmation")
	}
	approved, err := delivery.confirm(ctx, selected)
	if err != nil {
		return fmt.Errorf("confirm command execution: %w", err)
	}
	if !approved {
		return errors.New("command execution was not approved")
	}
	return nil
}

func deliverInjectCommand(delivery queryDelivery, selected *domain.Command, outcome *historyservice.SelectionOutcome) error {
	localAssessment := security.ValidateCommandForShell(selected.Command, terminal.DefaultShell())
	if localAssessment.Dangerous {
		selected.IsDangerous = true
		if selected.DangerousExplanation == "" {
			selected.DangerousExplanation = localAssessment.Explanation
		}
	}
	if err := writeQueryWarning(delivery.errOut, selected); err != nil {
		return recordDeliveryFailure(outcome, err)
	}
	commandToInject := selected.Command
	if delivery.wrapInject {
		var err error
		commandToInject, err = injectedRunInvocation(selected.Command, delivery.sessionID)
		if err != nil {
			return recordDeliveryFailure(outcome, err)
		}
	}
	inject := delivery.inject
	if inject == nil {
		inject = terminal.Inject
	}
	if err := inject(commandToInject); err != nil {
		delivery.logger.Debug("Failed to inject command into terminal", "error", err)
		var partial *terminal.PartialInjectionError
		if errors.As(err, &partial) && partial.Written > 0 {
			outcome.DeliveryOutcome = queryDeliveryUnknown
			outcome.ExecutionError = err.Error()
			if warningErr := writeQueryOutput(delivery.errOut, locale.Text(locale.KeyQueryWarningMessage), "Terminal injection was partial; do not submit the terminal input and inspect it before continuing."); warningErr != nil {
				return errors.Join(err, warningErr)
			}
			return fmt.Errorf("partial terminal injection: %w", err)
		}
		if err := writeQueryOutput(delivery.out, locale.Text(locale.KeyQuerySelectedCommandMessage), commandToInject); err != nil {
			return recordDeliveryFailure(outcome, err)
		}
		outcome.DeliveryOutcome = queryDeliveryPrinted
		return nil
	}
	outcome.DeliveryOutcome = "injected"
	return nil
}

func recordDeliveryFailure(outcome *historyservice.SelectionOutcome, err error) error {
	outcome.DeliveryOutcome = historyStatusFailed
	outcome.ExecutionError = err.Error()
	return err
}

func writeQueryWarning(out io.Writer, selected *domain.Command) error {
	if !selected.IsDangerous {
		return nil
	}
	return writeQueryOutput(out, locale.Text(locale.KeyQueryWarningMessage), selected.DangerousExplanation)
}

func confirmQueryExecution(cmd *cobra.Command) func(context.Context, *domain.Command) (bool, error) {
	return func(ctx context.Context, selected *domain.Command) (bool, error) {
		return prompt.ConfirmContext(ctx, prompt.ApprovalOptions{
			Prompt: fmt.Sprintf("command=%s\nexplanation=%s", strconv.QuoteToASCII(selected.Command), strconv.QuoteToASCII(selected.DangerousExplanation)),
			Output: cmd.ErrOrStderr(),
		})
	}
}

func writeQueryOutput(out io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(out, format, args...); err != nil {
		return fmt.Errorf("write query output: %w", err)
	}
	return nil
}
