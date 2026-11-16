package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	aliasrepo "github.com/ra1phdd/zev/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev/internal/entities/alias/service"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	"github.com/ra1phdd/zev/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	scriptrepo "github.com/ra1phdd/zev/internal/entities/script/repository"
	scriptservice "github.com/ra1phdd/zev/internal/entities/script/service"
	"github.com/ra1phdd/zev/pkg/terminal"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
	"github.com/spf13/cobra"
)

const (
	queryCancelled    = "cancelled"
	queryUserRole     = "user"
	queryRefineMarker = "::REFINE_QUERY::"
)

var (
	executeFlag       bool
	injectFlag        bool
	errQueryCancelled = errors.New(queryCancelled)
)

// parsedFlags contains manually parsed CLI flags without changing package state.
type parsedFlags struct {
	Args      []string
	Help      bool
	Verbosity int
	Execute   bool
	Inject    bool
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

func newQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query [query]",
		Short: "Query for command suggestions",
		Args:  cobra.ArbitraryArgs,
		RunE:  runQuery,
	}
	cmd.Flags().BoolVar(&executeFlag, "execute", false, "Execute selected command immediately")
	cmd.Flags().BoolVar(&injectFlag, "inject", false, "Inject selected command into terminal (default)")
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
func parseArgsFlags(args []string) parsedFlags {
	result := parsedFlags{Args: []string{}}
	for _, arg := range args {
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
		default:
			result.Args = append(result.Args, arg)
		}
	}
	return result
}

func runQuery(cmd *cobra.Command, args []string) error {
	parsed := parseArgsFlags(args)
	if parsed.Help {
		if err := cmd.Help(); err != nil {
			return fmt.Errorf("show query help: %w", err)
		}
		return nil
	}
	// The root command parses flags manually to preserve alias arguments.
	verbosityLevel, executeFlag, injectFlag = parsed.Verbosity, parsed.Execute, parsed.Inject
	logger := setupLogger(verbosityLevel)
	err := runConfiguredQuery(cmd, parsed.Args, logger)
	if errors.Is(err, errQueryCancelled) {
		return nil
	}
	return err
}

func runConfiguredQuery(cmd *cobra.Command, args []string, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w. Run `zev setup` to configure", err)
	}
	logger.Info("Configuration loaded", "provider", cfg.LLMProvider)
	userPrompt := strings.Join(args, " ")
	if len(args) == 0 {
		var err error
		userPrompt, err = readQueryInput(prompt.Input, "What command do you need?")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(userPrompt) == "" {
		return errors.New("prompt cannot be empty")
	}
	if handled, err := executeNamedQuery(cmd.Context(), userPrompt, logger); handled {
		return err
	}
	httpClient := httpclient.New(cfg.HTTPHeaders, logger)
	provider, err := factory.NewProvider(cfg, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}
	logger.Debug("Provider created", "name", provider.Name(), "model", provider.Model())
	s := newQuerySession(queryservice.New(provider), cmd, userPrompt)
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
	if err := deliverQueryCommand(cmd.Context(), queryDelivery{
		command: selected, mode: mode, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), logger: logger,
	}); err != nil {
		return err
	}
	entry := service.HistoryEntry{
		Timestamp: time.Now(), Prompt: s.userPrompt, Response: s.response,
		Provider: provider.Name(), Model: provider.Model(),
		SelectedCommand: selected.Command, SelectedIndex: index,
	}
	if err := service.New().Save(entry); err != nil {
		return writeQueryOutput(cmd.ErrOrStderr(), "Warning: failed to save to history: %v\n", err)
	}
	logger.Info("Saved to history")
	return nil
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
	svc        *queryservice.Service
	cmd        *cobra.Command
	history    *domain.ConversationHistory
	userPrompt string
	response   *domain.OptionsResponse
	input      func(string) (string, error)
	selectItem func(list.SelectOptions) (list.Item, error)
}

func newQuerySession(svc *queryservice.Service, cmd *cobra.Command, userPrompt string) *querySession {
	return &querySession{
		svc: svc, cmd: cmd, userPrompt: userPrompt,
		history: &domain.ConversationHistory{
			SystemPrompt: fmt.Sprintf(repository.SystemPrompt, svc.GetEnvContext()),
			Messages:     []domain.Message{{Role: queryUserRole, Content: userPrompt}},
		},
		input: prompt.Input, selectItem: list.Select,
	}
}

func readQueryInput(input func(string) (string, error), title string) (string, error) {
	value, err := input(title)
	if err != nil {
		if err.Error() == queryCancelled {
			return "", errQueryCancelled
		}
		return "", fmt.Errorf("get query input: %w", err)
	}
	return value, nil
}

func (s *querySession) fetch(ctx context.Context) error {
	if err := writeQueryOutput(s.cmd.OutOrStdout(), "Fetching suggestions...\n"); err != nil {
		return err
	}
	response, err := s.svc.GetOptionsWithHistory(ctx, s.history)
	if err != nil {
		return fmt.Errorf("get command suggestions: %w", err)
	}
	s.response = response
	return nil
}

func (s *querySession) start(ctx context.Context) error {
	err := s.fetch(ctx)
	if !errors.Is(err, domain.ErrNoResponse) {
		return err
	}
	if err := writeQueryOutput(s.cmd.ErrOrStderr(), "\n⚠️  The AI did not provide a response. This might happen if your request is unclear.\n\n"); err != nil {
		return err
	}
	refined, err := readQueryInput(s.input, "Please clarify your request:")
	if err != nil {
		return err
	}
	if strings.TrimSpace(refined) == "" {
		return errors.New("the request cannot be empty")
	}
	s.addRefinement(refined)
	return s.fetch(ctx)
}

func (s *querySession) addRefinement(value string) {
	s.userPrompt = value
	s.history.Messages = append(s.history.Messages, domain.Message{Role: queryUserRole, Content: value})
}

func (s *querySession) acceptResponse() (bool, error) {
	if !s.response.IsValid {
		return false, writeQueryOutput(s.cmd.ErrOrStderr(), "\n⚠️  Unable to process your request:\n%s\n", s.response.ExplanationIfNotValid)
	}
	if len(s.response.Commands) == 0 {
		return false, writeQueryOutput(s.cmd.OutOrStdout(), "\n⚠️  No commands found for your query.\n")
	}
	s.history.Messages = append(s.history.Messages, domain.Message{Role: "assistant", Content: s.response.CompactJSON()})
	return true, nil
}

func (s *querySession) commandItems() []list.Item {
	items := make([]list.Item, 0, len(s.response.Commands)+1)
	for i := range s.response.Commands {
		items = append(items, list.NewCommandItem(&s.response.Commands[i]))
	}
	items = append(items, list.NewCommandItem(&domain.Command{
		Command: queryRefineMarker, ShortExplanation: "Clarify the request",
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
		Title: "Select a command to run:", Items: s.commandItems(), ShowWarning: true,
	})
	if err != nil {
		if err.Error() == queryCancelled {
			return nil, errQueryCancelled
		}
		return nil, fmt.Errorf("failed to select command: %w", err)
	}
	item, ok := selected.(list.CommandItem)
	if !ok || item.Cmd == nil {
		return nil, fmt.Errorf("unexpected command selection %T", selected)
	}
	return item.Cmd, nil
}

func (s *querySession) refine(ctx context.Context) (bool, error) {
	refined, err := readQueryInput(s.input, "Specify your request:")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(refined) == "" {
		return true, writeQueryOutput(s.cmd.ErrOrStderr(), "The request cannot be empty.\n")
	}
	s.addRefinement(refined)
	err = s.fetch(ctx)
	if errors.Is(err, domain.ErrNoResponse) {
		return true, writeQueryOutput(s.cmd.ErrOrStderr(), "\n⚠️  The AI did not provide a response. Please try rephrasing your request.\n")
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
}

func deliverQueryCommand(ctx context.Context, delivery queryDelivery) error {
	selected := delivery.command
	if delivery.mode == executionModeExecute {
		if err := terminal.Execute(ctx, selected.Command); err != nil {
			return fmt.Errorf("failed to execute command: %w", err)
		}
		return writeQueryWarning(delivery.errOut, selected)
	}
	if err := terminal.Inject(selected.Command); err != nil {
		delivery.logger.Debug("Failed to inject command into terminal", "error", err)
		if err := writeQueryOutput(delivery.out, "\n✓ Selected command:\n\n  %s\n\n", selected.Command); err != nil {
			return err
		}
		if selected.IsDangerous {
			return writeQueryOutput(delivery.out, "⚠️  WARNING: %s\n\n", selected.DangerousExplanation)
		}
		return nil
	}
	return writeQueryWarning(delivery.errOut, selected)
}

func writeQueryWarning(out io.Writer, selected *domain.Command) error {
	if !selected.IsDangerous {
		return nil
	}
	return writeQueryOutput(out, "\n⚠️  WARNING: %s\n", selected.DangerousExplanation)
}

func writeQueryOutput(out io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(out, format, args...); err != nil {
		return fmt.Errorf("write query output: %w", err)
	}
	return nil
}
