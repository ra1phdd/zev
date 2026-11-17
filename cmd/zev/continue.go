package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	historyrepo "github.com/ra1phdd/zev/internal/entities/history/repository"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	"github.com/ra1phdd/zev/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
	"github.com/spf13/cobra"
)

type continuationInputs struct {
	ContextFiles  []string
	CommandLog    string
	Clarification string
}

func collectContinuationInputs(input func(string) (string, error), explicitFiles []string, interactive bool) (continuationInputs, error) {
	return collectContinuationInputsWithLog(input, input, explicitFiles, interactive)
}

func collectContinuationInputsWithLog(input, logInput func(string) (string, error), explicitFiles []string, interactive bool) (continuationInputs, error) {
	result := continuationInputs{ContextFiles: append([]string(nil), explicitFiles...)}
	if !interactive {
		return result, nil
	}
	if len(result.ContextFiles) == 0 {
		value, err := readQueryInput(input, "Attach context files (comma-separated paths), or press Enter to skip:")
		if err != nil {
			return result, err
		}
		result.ContextFiles = splitContinuationPaths(value)
	}
	value, err := logInput("Paste the command log (Enter: newline, Ctrl+D: next or skip):")
	if err != nil {
		return result, err
	}
	result.CommandLog = value
	result.Clarification, err = readQueryInput(input, "Describe how to continue, or press Enter to skip:")
	if err != nil {
		return result, err
	}
	if err := validateContinuationInputs(result); err != nil {
		return result, err
	}
	return result, nil
}

func validateContinuationInputs(inputs continuationInputs) error {
	if len(inputs.ContextFiles) == 0 && strings.TrimSpace(inputs.CommandLog) == "" && strings.TrimSpace(inputs.Clarification) == "" {
		return errors.New("continuation requires context files, a command log, or a clarification")
	}
	return nil
}

func splitContinuationPaths(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' })
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if path := strings.TrimSpace(part); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func continuationMessage(kind, content string) domain.Message {
	return domain.Message{Role: queryUserRole, Kind: kind, Content: content}
}

func closeContinuationStore(store *historyrepo.Store, logger *slog.Logger) {
	if err := store.Close(); err != nil {
		logger.Warn("failed to close continuation history", "error", err)
	}
}

//nolint:gocognit // Continuation orchestration keeps preflight, provider setup, persistence, and delivery in one fail-closed path.
func runContinuationQuery(cmd *cobra.Command, flags parsedFlags, logger *slog.Logger) (resultErr error) {
	if flags.NoHistory {
		return errors.New("--continue requires history; remove --no-history")
	}
	store, err := openQueryHistory(cmd.Context())
	if err != nil {
		return fmt.Errorf("open continuation history: %w", err)
	}
	view, err := store.LastConversation(cmd.Context())
	if err != nil {
		closeContinuationStore(store, logger)
		return fmt.Errorf("find continuation history: %w", err)
	}
	seed, err := historyservice.RestoreConversation(view)
	if err != nil {
		closeContinuationStore(store, logger)
		return fmt.Errorf("restore continuation history: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		closeContinuationStore(store, logger)
		return fmt.Errorf("failed to load config: %w. Run `zev setup` to configure", err)
	}
	if !cfg.HistoryEnabled {
		closeContinuationStore(store, logger)
		return errors.New("continuation requires HISTORY_ENABLED=true")
	}
	interactive := !flags.Print && !flags.JSON && len(flags.Args) == 0
	inputs, err := collectContinuationInputsWithLog(func(title string) (string, error) {
		return prompt.InputContext(cmd.Context(), title)
	}, func(title string) (string, error) {
		return prompt.MultilineInputContext(cmd.Context(), title)
	}, flags.ContextFiles, interactive)
	if err != nil {
		closeContinuationStore(store, logger)
		return err
	}
	projectContext, err := loadProjectContext(inputs.ContextFiles, flags.AllowSensitiveContext)
	if err != nil {
		closeContinuationStore(store, logger)
		return err
	}
	clarification := strings.Join(flags.Args, " ")
	if interactive {
		clarification = inputs.Clarification
	}
	if strings.TrimSpace(clarification) == "" && strings.TrimSpace(inputs.CommandLog) == "" && projectContext == "" {
		closeContinuationStore(store, logger)
		return errors.New("continuation requires context files, a command log, or a clarification")
	}
	promptText := clarification
	if strings.TrimSpace(promptText) == "" {
		promptText = "Continue using the supplied context."
	}
	provider, err := factory.NewProvider(cfg, httpclient.New(cfg.HTTPHeaders, logger))
	if err != nil {
		closeContinuationStore(store, logger)
		return fmt.Errorf("failed to create provider: %w", err)
	}
	envContext := queryservice.New(provider).GetEnvContext()
	if !cfg.ContextEnabled || flags.NoContext {
		envContext = queryLocalContextDisabled
	}
	s := newQuerySession(queryservice.New(provider), cmd, promptText)
	s.machine = flags.Print || flags.JSON
	s.historyStore, s.persistence = store, historyservice.NewLifecycle(store)
	s.provider, s.model = provider.Name(), provider.Model()
	s.startHistory = append([]domain.Message(nil), seed...)
	if projectContext != "" {
		s.continuationMessages = append(s.continuationMessages, continuationMessage("project_context", projectContext))
	}
	if strings.TrimSpace(inputs.CommandLog) != "" {
		s.continuationMessages = append(s.continuationMessages, continuationMessage("command_error", inputs.CommandLog))
	}
	if strings.TrimSpace(clarification) != "" || promptText != "" {
		s.continuationMessages = append(s.continuationMessages, continuationMessage("refinement", promptText))
	}
	s.history = &domain.ConversationHistory{SystemPrompt: fmt.Sprintf(repository.SystemPrompt, envContext), Messages: append([]domain.Message(nil), seed...)}
	s.history.Messages = append(s.history.Messages, s.continuationMessages...)
	defer s.closeHistory(cmd.Context(), &resultErr)
	if err := s.start(cmd.Context()); err != nil {
		return err
	}
	ready, err := s.acceptResponse()
	if err != nil || !ready {
		return err
	}
	if s.machine {
		if err := writeMachineResult(cmd, flags, s.response); err != nil {
			return err
		}
		s.warnHistory(s.persistence.Select(cmd.Context(), historyservice.SelectionOutcome{Index: flags.Option - 1, DeliveryOutcome: queryDeliveryPrinted}))
		return nil
	}
	mode := resolveExecutionMode(cfg, executeFlag, injectFlag)
	selected, index, err := s.selectCommand(cmd.Context(), mode)
	if err != nil || selected == nil {
		return err
	}
	outcome := historyservice.SelectionOutcome{Index: index, DeliveryOutcome: queryDeliverySelected}
	s.warnHistory(s.persistence.Select(cmd.Context(), outcome))
	err = deliverQueryCommand(cmd.Context(), queryDelivery{command: selected, mode: mode, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), logger: logger, outcome: &outcome})
	ctx, cancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 5*time.Second)
	s.warnHistory(s.persistence.Select(ctx, outcome))
	cancel()
	return err
}
