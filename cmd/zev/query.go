package main

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
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	historyrepo "github.com/ra1phdd/zev/internal/entities/history/repository"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	"github.com/ra1phdd/zev/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	scriptservice "github.com/ra1phdd/zev/internal/entities/script/service"
	"github.com/ra1phdd/zev/locale"
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
	AllowSensitiveContext bool
	ContextFiles          []string
	Print                 bool
	JSON                  bool
	Option                int
	OptionSet             bool
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
	return &cobra.Command{
		Use:                "continue [clarification]",
		Short:              locale.Text("Continue the latest usable conversation"),
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(cmd, append([]string{"--continue"}, args...))
		},
	}
}

func newQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query [query]",
		Short: locale.Text("Query for command suggestions"),
		Args:  cobra.ArbitraryArgs,
		RunE:  runQuery,
	}
	cmd.Flags().BoolVar(&executeFlag, "execute", false, locale.Text("Execute selected command immediately"))
	cmd.Flags().BoolVar(&injectFlag, "inject", false, locale.Text("Inject selected command into terminal (default)"))
	cmd.Flags().BoolVar(&noHistoryFlag, "no-history", false, locale.Text("Do not persist this query in history"))
	cmd.Flags().BoolVar(&noContextFlag, "no-context", false, locale.Text("Do not send local environment context"))
	cmd.DisableFlagParsing = true
	cmd.Flags().Bool("print", false, locale.Text("Print the chosen command without executing it"))
	cmd.Flags().Bool("json", false, locale.Text("Output command options as schema version 1 JSON"))
	cmd.Flags().Int("option", 1, locale.Text("Choose a 1-based command option in machine mode"))
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
		case arg == "-h", arg == "--help":
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
		case arg == "--continue":
			result.Continue = true
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
	parsed := parseArgsFlags(args)
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
	err := runConfiguredQuery(cmd, parsed, logger)
	if errors.Is(err, errQueryCancelled) {
		return nil
	}
	return err
}

//nolint:gocognit // Existing query orchestration owns configuration, provider setup, history, and delivery.
func runConfiguredQuery(cmd *cobra.Command, flags parsedFlags, logger *slog.Logger) (resultErr error) {
	if flags.Continue {
		return runContinuationQuery(cmd, flags, logger)
	}
	args := flags.Args
	projectContext, err := loadProjectContext(flags.ContextFiles, flags.AllowSensitiveContext)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf(locale.Text("failed to load config: %w. Run `zev setup` to configure"), err)
	}
	cfg.HistoryEnabled = cfg.HistoryEnabled && !flags.NoHistory
	cfg.ContextEnabled = cfg.ContextEnabled && !flags.NoContext
	logger.Info("Configuration loaded", "provider", cfg.LLMProvider)
	userPrompt := strings.Join(args, " ")
	if len(args) == 0 {
		var err error
		userPrompt, err = readQueryInput(func(title string) (string, error) {
			return prompt.InputContext(cmd.Context(), title)
		}, locale.Text("What command do you need?"))
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(userPrompt) == "" {
		return errors.New(locale.Text("prompt cannot be empty"))
	}
	if handled, err := executeNamedQuery(cmd.Context(), userPrompt, logger); handled {
		return err
	}
	httpClient := httpclient.New(cfg.HTTPHeaders, logger)
	provider, err := factory.NewProvider(cfg, httpClient)
	if err != nil {
		return fmt.Errorf(locale.Text("failed to create provider: %w"), err)
	}
	logger.Debug("Provider created", "name", provider.Name(), "model", provider.Model())
	s := newQuerySessionWithProjectContext(queryservice.New(provider), cmd, userPrompt, projectContext)
	s.provider, s.model = provider.Name(), provider.Model()
	s.prepareHistory(cfg.HistoryEnabled)
	defer s.closeHistory(cmd.Context(), &resultErr)
	if !cfg.ContextEnabled {
		contextPrompt := queryLocalContextDisabled
		if projectContext != "" {
			contextPrompt += "\n\n" + projectContextSection(projectContext)
		}
		s.history.SystemPrompt = fmt.Sprintf(repository.SystemPrompt, contextPrompt)
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
	err = deliverQueryCommand(cmd.Context(), queryDelivery{
		command: selected, mode: mode, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), logger: logger, outcome: &outcome,
	})
	if s.persistence != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 5*time.Second)
		s.warnHistory(s.persistence.Select(ctx, outcome))
		cancel()
	}
	return err
}

func openQueryHistory(ctx context.Context) (*historyrepo.Store, error) {
	dir := config.GetZevDir()
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, fmt.Errorf("validate history directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	store, err := historyrepo.Open(ctx, filepath.Join(dir, "history.db"))
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	return store, nil
}

func executeNamedQuery(ctx context.Context, userPrompt string, logger *slog.Logger) (bool, error) {
	parts := strings.Fields(userPrompt)
	name, args := parts[0], parts[1:]
	aliasSvc := aliasservice.New(aliasrepo.New(config.GetZevDir()))
	resolved, err := aliasSvc.Resolve(name, args)
	if err == nil {
		logger.Debug("Alias resolved", "name", name)
		if err := terminal.Execute(ctx, resolved); err != nil {
			return true, fmt.Errorf("execute alias: %w", err)
		}
		return true, nil
	}
	if err != nil && !errors.Is(err, aliasservice.ErrAliasNotFound) {
		return true, fmt.Errorf("resolve alias: %w", err)
	}
	scriptSvc := scriptservice.New(scriptrepo.New(config.GetZevDir()))
	if err := scriptSvc.Execute(ctx, name, args); err != nil {
		if errors.Is(err, scriptservice.ErrScriptNotFound) {
			return false, nil
		}
		return true, fmt.Errorf("execute script: %w", err)
	}
	return true, nil
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
}

func newQuerySessionWithProjectContext(svc *queryservice.Service, cmd *cobra.Command, userPrompt, projectContext string) *querySession {
	ctx := svc.GetEnvContext()
	if projectContext != "" {
		ctx += "\n\n" + projectContextSection(projectContext)
	}
	return &querySession{
		svc: svc, cmd: cmd, userPrompt: userPrompt, projectContext: projectContext,
		history: &domain.ConversationHistory{
			SystemPrompt: fmt.Sprintf(repository.SystemPrompt, ctx),
			Messages:     []domain.Message{{Role: queryUserRole, Content: userPrompt}},
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
		return "", fmt.Errorf(locale.Text("get query input: %w"), err)
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
		if _, writeErr := fmt.Fprintf(s.cmd.ErrOrStderr(), locale.Text("Warning: failed to save to history: %v\n"), err); writeErr != nil {
			return
		}
	}
}

func (s *querySession) fetch(ctx context.Context) error {
	progressOut := s.cmd.OutOrStdout()
	if s.machine {
		progressOut = s.cmd.ErrOrStderr()
	}
	if err := writeQueryOutput(progressOut, "%s", locale.Text("Fetching suggestions...\n")); err != nil {
		return err
	}
	response, err := s.svc.GetOptionsWithHistory(ctx, s.history)
	if err != nil {
		return fmt.Errorf(locale.Text("get command suggestions: %w"), err)
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
	if err := writeQueryOutput(s.cmd.ErrOrStderr(), "%s", locale.Text("\n⚠️  The AI did not provide a response. This might happen if your request is unclear.\n\n")); err != nil {
		return err
	}
	refined, err := readQueryInput(s.input, locale.Text("Please clarify your request:"))
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
	s.userPrompt = value
	s.history.Messages = append(s.history.Messages, domain.Message{Role: queryUserRole, Content: value})
}

func (s *querySession) acceptResponse() (bool, error) {
	if err := s.response.Validate(); err != nil {
		if s.machine {
			return false, machineError(4, err)
		}
		return false, fmt.Errorf("validate response before selection: %w", err)
	}
	if !s.response.IsValid {
		if s.machine {
			return false, machineError(4, fmt.Errorf(locale.Text("provider returned an invalid response: %s"), s.response.ExplanationIfNotValid))
		}
		return false, writeQueryOutput(s.cmd.ErrOrStderr(), locale.Text("\n⚠️  Unable to process your request:\n%s\n"), s.response.ExplanationIfNotValid)
	}
	if len(s.response.Commands) == 0 {
		if s.machine {
			return false, machineError(4, errors.New(locale.Text("provider returned no commands")))
		}
		return false, writeQueryOutput(s.cmd.OutOrStdout(), "%s", locale.Text("\n⚠️  No commands found for your query.\n"))
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
		Command: queryRefineMarker, ShortExplanation: locale.Text("Clarify the request"),
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
		Title: locale.Text("Select a command to run:"), Items: s.commandItems(), ShowWarning: true,
	})
	if err != nil {
		if err.Error() == queryCancelled {
			return nil, errQueryCancelled
		}
		return nil, fmt.Errorf(locale.Text("failed to select command: %w"), err)
	}
	item, ok := selected.(list.CommandItem)
	if !ok || item.Cmd == nil {
		return nil, fmt.Errorf("unexpected command selection %T", selected)
	}
	return item.Cmd, nil
}

func (s *querySession) refine(ctx context.Context) (bool, error) {
	refined, err := readQueryInput(s.input, locale.Text("Specify your request:"))
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(refined) == "" {
		return true, writeQueryOutput(s.cmd.ErrOrStderr(), "%s", locale.Text("The request cannot be empty.\n"))
	}
	s.addRefinement(refined)
	if s.persistence != nil {
		s.warnHistory(s.persistence.Refine(ctx, refined))
	}
	err = s.fetch(ctx)
	if errors.Is(err, domain.ErrNoResponse) {
		return true, writeQueryOutput(s.cmd.ErrOrStderr(), "%s", locale.Text("\n⚠️  The AI did not provide a response. Please try rephrasing your request.\n"))
	}
	if err != nil {
		return false, err
	}
	return s.acceptResponse()
}

type queryDelivery struct {
	command *domain.Command
	mode    string
	out     io.Writer
	errOut  io.Writer
	logger  *slog.Logger
	outcome *historyservice.SelectionOutcome
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
		outcome.DeliveryOutcome = "unknown"
		outcome.ExecutionError = err.Error()
		return fmt.Errorf("deliver selected command: %w", err)
	}
	if delivery.mode == executionModeExecute {
		if err := terminal.Execute(ctx, selected.Command); err != nil {
			outcome.DeliveryOutcome = "unknown"
			outcome.ExecutionError = err.Error()
			if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
				code := exitErr.ExitCode()
				outcome.DeliveryOutcome = "failed"
				outcome.ExecutionExitCode = &code
			}
			return fmt.Errorf(locale.Text("failed to execute command: %w"), err)
		}
		now := time.Now()
		code := 0
		outcome.DeliveryOutcome = "executed"
		outcome.ExecutedAt = &now
		outcome.ExecutionExitCode = &code
		return writeQueryWarning(delivery.errOut, selected)
	}
	if err := terminal.Inject(selected.Command); err != nil {
		delivery.logger.Debug("Failed to inject command into terminal", "error", err)
		if err := writeQueryOutput(delivery.out, locale.Text("\n✓ Selected command:\n\n  %s\n\n"), selected.Command); err != nil {
			outcome.DeliveryOutcome = "failed"
			outcome.ExecutionError = err.Error()
			return err
		}
		outcome.DeliveryOutcome = queryDeliveryPrinted
		if selected.IsDangerous {
			return writeQueryOutput(delivery.out, locale.Text("⚠️  WARNING: %s\n\n"), selected.DangerousExplanation)
		}
		return nil
	}
	outcome.DeliveryOutcome = "injected"
	return writeQueryWarning(delivery.errOut, selected)
}

func writeQueryWarning(out io.Writer, selected *domain.Command) error {
	if !selected.IsDangerous {
		return nil
	}
	return writeQueryOutput(out, locale.Text("\n⚠️  WARNING: %s\n"), selected.DangerousExplanation)
}

func writeQueryOutput(out io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(out, format, args...); err != nil {
		return fmt.Errorf("write query output: %w", err)
	}
	return nil
}
