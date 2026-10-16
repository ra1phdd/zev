package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/core/httpclient"
	aliasrepo "github.com/ra1phdd/zev-go/internal/entities/alias/repository"
	aliasservice "github.com/ra1phdd/zev-go/internal/entities/alias/service"
	"github.com/ra1phdd/zev-go/internal/entities/history/service"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev-go/internal/entities/query/service"
	scriptrepo "github.com/ra1phdd/zev-go/internal/entities/script/repository"
	scriptservice "github.com/ra1phdd/zev-go/internal/entities/script/service"
	"github.com/ra1phdd/zev-go/pkg/terminal"
	"github.com/ra1phdd/zev-go/pkg/tui/list"
	"github.com/ra1phdd/zev-go/pkg/tui/prompt"
	"github.com/spf13/cobra"
)

var (
	executeFlag bool
	injectFlag  bool
)

// setupLogger настраивает логгер в зависимости от уровня verbosity.
// 0 = FATAL (по умолчанию)
// -v (1) = INFO
// -vv (2) = DEBUG
// -vvv (3+) = DEBUG с source location.
func setupLogger(verbosity int) *slog.Logger {
	var level slog.Level
	var addSource bool

	switch verbosity {
	case 0:
		level = slog.Level(10)
	case 1:
		level = slog.LevelInfo
	case 2:
		level = slog.LevelDebug
	default: // >= 3
		level = slog.LevelDebug
		addSource = true
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: addSource,
	}

	handler := slog.NewTextHandler(os.Stderr, opts)
	return slog.New(handler)
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

// resolveExecutionMode определяет режим выполнения команды на основе флагов и конфига.
// Приоритет: флаги > config > дефолт (inject).
func resolveExecutionMode(cfg *config.Config, executeFlag, injectFlag bool) string {
	if executeFlag {
		return "execute"
	}
	if injectFlag {
		return "inject"
	}
	if cfg.CommandExecutionMode != "" {
		return cfg.CommandExecutionMode
	}
	return "inject"
}

// parseArgsFlags разбирает флаги zev вручную, т.к. на корневом комманде
// установлен DisableFlagParsing ради поддержки алиасов вида `zev dps -a`.
// Поддерживаются: -h/--help, -v/-vv/-vvv/... (счётчик), --verbose=N,
// -e/--execute, --inject. Всё остальное считается аргументами пользователя.
func parseArgsFlags(args []string) ([]string, bool) {
	filteredArgs := []string{}
	helpRequested := false

	for _, arg := range args {
		switch {
		case arg == "-h", arg == "--help":
			helpRequested = true

		case arg == "-v":
			verbosityLevel++

		case arg == "--verbose":
			verbosityLevel++

		case strings.HasPrefix(arg, "--verbose="):
			if n, err := strconv.Atoi(strings.TrimPrefix(arg, "--verbose=")); err == nil && n > 0 {
				verbosityLevel = n
			}

		case len(arg) > 2 && strings.HasPrefix(arg, "-vv"):
			// -vv, -vvv, -vvvv... — количество v задаёт уровень verbosity
			verbosityLevel = len(arg) - 1

		case arg == "-e", arg == "--execute":
			executeFlag = true

		case arg == "--inject":
			injectFlag = true

		default:
			filteredArgs = append(filteredArgs, arg)
		}
	}

	return filteredArgs, helpRequested
}

func runQuery(cmd *cobra.Command, args []string) error {
	// Обработка флагов вручную (т.к. DisableFlagParsing=true для поддержки алиасов с флагами)
	filteredArgs, helpRequested := parseArgsFlags(args)
	if helpRequested {
		return cmd.Help()
	}
	args = filteredArgs

	logger := setupLogger(verbosityLevel)
	logger.Debug("Starting query command", "args", args, "os.Args", os.Args, "verbosity", verbosityLevel)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w. Run `zev setup` to configure", err)
	}
	logger.Info("Configuration loaded", "provider", cfg.LLMProvider)

	var userPrompt string
	if len(args) > 0 {
		userPrompt = strings.Join(args, " ")
	} else {
		userPrompt, err = prompt.Input("What command do you need?")
		if err != nil {
			if err.Error() == "cancelled" {
				return nil
			}
			return fmt.Errorf("failed to get input: %w", err)
		}
	}

	if strings.TrimSpace(userPrompt) == "" {
		return errors.New("prompt cannot be empty")
	}

	// Проверяем алиасы
	parts := strings.Fields(userPrompt)
	if len(parts) > 0 {
		aliasName := parts[0]
		aliasArgs := parts[1:]

		aliasRepo := aliasrepo.New()
		aliasSvc := aliasservice.New(aliasRepo)

		resolvedCmd, err := aliasSvc.Resolve(aliasName, aliasArgs)
		if err == nil {
			// Алиас найден — выполняем команду
			logger.Debug("Alias resolved", "name", aliasName, "command", resolvedCmd)
			return terminal.Execute(cmd.Context(), resolvedCmd)
		}
		logger.Debug("Alias not found, checking scripts", "name", aliasName, "error", err)

		// Проверяем скрипты
		scriptRepo := scriptrepo.New()
		scriptSvc := scriptservice.New(scriptRepo)

		err = scriptSvc.Execute(aliasName, aliasArgs)
		if !errors.Is(err, scriptservice.ErrScriptNotFound) {
			// Скрипт найден — возвращаем его результат (в т.ч. ненулевой exit code)
			logger.Debug("Script executed", "name", aliasName, "error", err)
			return err
		}
		logger.Debug("Script not found, falling back to LLM query", "name", aliasName)
	}

	// Fallback на LLM запрос

	httpClient := httpclient.New(cfg.HTTPHeaders)
	provider, err := factory.NewProvider(cfg, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}
	logger.Debug("Provider created", "name", provider.Name(), "model", provider.Model())

	querySvc := queryservice.New(provider)
	ctx := context.Background()

	history := &domain.ConversationHistory{
		SystemPrompt: repository.SystemPrompt,
		Messages:     []domain.Message{},
	}

	envContext := querySvc.GetEnvContext()
	firstUserMessage := fmt.Sprintf(repository.UserPromptTemplate, envContext, userPrompt)
	history.Messages = append(history.Messages, domain.Message{
		Role:    "user",
		Content: firstUserMessage,
	})

	logger.Debug("Sending request to LLM", "prompt_length", len(userPrompt))
	fmt.Fprintln(os.Stdout, "Fetching suggestions...")
	response, err := provider.GetOptionsWithHistory(ctx, history)
	if err != nil {
		logger.Error("Failed to get response from provider", "error", err)
		if errors.Is(err, domain.ErrNoResponse) {
			fmt.Fprintf(os.Stderr, "\n⚠️  The AI did not provide a response. This might happen if your request is unclear.\n\n")
			refinedPrompt, promptErr := prompt.Input("Please clarify your request:")
			if promptErr != nil {
				if promptErr.Error() == "cancelled" {
					return nil
				}
				return fmt.Errorf("failed to get refined input: %w", promptErr)
			}

			if strings.TrimSpace(refinedPrompt) == "" {
				return errors.New("the request cannot be empty")
			}

			userPrompt = refinedPrompt
			logger.Debug("User clarified query after no response", "refined_prompt", refinedPrompt)

			history.Messages = append(history.Messages, domain.Message{
				Role:    "user",
				Content: refinedPrompt,
			})

			fmt.Fprintln(os.Stdout, "Fetching suggestions...")
			response, err = provider.GetOptionsWithHistory(ctx, history)
			if err != nil {
				logger.Error("Failed to get response after clarification", "error", err)
				return err
			}
		} else {
			return err
		}
	}
	logger.Info("Received response", "commands_count", len(response.Commands), "is_valid", response.IsValid)

	if !response.IsValid {
		fmt.Fprintf(os.Stderr, "\n⚠️  Unable to process your request:\n%s\n", response.ExplanationIfNotValid)
		return nil
	}

	if len(response.Commands) == 0 {
		fmt.Fprintln(os.Stdout, "\n⚠️  No commands found for your query.")
		return nil
	}

	history.Messages = append(history.Messages, domain.Message{
		Role:    "assistant",
		Content: response.CompactJSON(),
	})

	mode := resolveExecutionMode(cfg, executeFlag, injectFlag)
	logger.Debug("Execution mode resolved", "mode", mode)

	const refineMarker = "::REFINE_QUERY::"
	commandsWithRefine := make([]domain.Command, len(response.Commands), len(response.Commands)+1)
	copy(commandsWithRefine, response.Commands)
	commandsWithRefine = append(commandsWithRefine, domain.Command{
		Command:          refineMarker,
		ShortExplanation: "Clarify the request",
		IsDangerous:      false,
	})

	var selectedCmd *domain.Command
	var selectedIndex int

	// В режиме execute сразу выполняем первую команду
	if mode == "execute" {
		selectedCmd = &response.Commands[0]
		selectedIndex = 0
		logger.Debug("Auto-selected first command in execute mode", "command", selectedCmd.Command)
	} else {
		// В режиме inject показываем селектор
		for {
			items := make([]list.Item, len(commandsWithRefine))
			for i := range commandsWithRefine {
				items[i] = list.NewCommandItem(&commandsWithRefine[i])
			}

			selected, err := list.Select(list.SelectOptions{
				Title:       "Select a command to run:",
				Items:       items,
				ShowWarning: true,
			})
			if err != nil {
				if err.Error() == "cancelled" {
					return nil
				}
				return fmt.Errorf("failed to select command: %w", err)
			}

			cmdItem := selected.(list.CommandItem)
			selectedCmd = cmdItem.Cmd

			if selectedCmd.Command == refineMarker {
					refinedPrompt, err := prompt.Input("Specify your request:")
				if err != nil {
					if err.Error() == "cancelled" {
						return nil
					}
					return fmt.Errorf("failed to get refined input: %w", err)
				}

				if strings.TrimSpace(refinedPrompt) == "" {
					fmt.Fprintln(os.Stderr, "The request cannot be empty.")
					continue
				}

				userPrompt = refinedPrompt
				logger.Debug("User refined query", "refined_prompt", refinedPrompt)

				history.Messages = append(history.Messages, domain.Message{
					Role:    "user",
					Content: refinedPrompt,
				})

				fmt.Fprintln(os.Stdout, "Fetching suggestions...")
				response, err = provider.GetOptionsWithHistory(ctx, history)
				if err != nil {
					logger.Error("Failed to get refined response", "error", err)
					if errors.Is(err, domain.ErrNoResponse) {
						fmt.Fprintf(os.Stderr, "\n⚠️  The AI did not provide a response. Please try rephrasing your request.\n")
						continue
					}
					return err
				}
				logger.Info("Received refined response", "commands_count", len(response.Commands))

				if !response.IsValid {
					fmt.Fprintf(os.Stderr, "\n⚠️  Unable to process your request:\n%s\n", response.ExplanationIfNotValid)
					return nil
				}

				if len(response.Commands) == 0 {
					fmt.Fprintln(os.Stdout, "\n⚠️  No commands found for your query.")
					return nil
				}

				history.Messages = append(history.Messages, domain.Message{
					Role:    "assistant",
					Content: response.CompactJSON(),
				})

				commandsWithRefine = make([]domain.Command, len(response.Commands), len(response.Commands)+1)
				copy(commandsWithRefine, response.Commands)
				commandsWithRefine = append(commandsWithRefine, domain.Command{
					Command:          refineMarker,
					ShortExplanation: "Clarify the request",
					IsDangerous:      false,
				})

				continue
			}

			for i := range response.Commands {
				if response.Commands[i].Command == selectedCmd.Command {
					selectedIndex = i
					break
				}
			}
			break
		}
	}

	logger.Debug("Selected command", "command", selectedCmd.Command, "is_dangerous", selectedCmd.IsDangerous)

	if mode == "execute" {
		if err := terminal.Execute(context.Background(), selectedCmd.Command); err != nil {
			logger.Error("Failed to execute command", "error", err)
			return fmt.Errorf("failed to execute command: %w", err)
		}
		if selectedCmd.IsDangerous {
			fmt.Fprintf(os.Stderr, "\n⚠️  WARNING: %s\n", selectedCmd.DangerousExplanation)
		}
	} else {
		if err := terminal.Inject(selectedCmd.Command); err != nil {
			// Если не удалось вставить (не TTY, unsupported platform, или ioctl failed), просто выводим команду
			logger.Debug("Failed to inject command into terminal", "error", err)
			fmt.Fprintf(os.Stdout, "\n✓ Selected command:\n\n  %s\n\n", selectedCmd.Command)
			if selectedCmd.IsDangerous {
				fmt.Fprintf(os.Stdout, "⚠️  WARNING: %s\n\n", selectedCmd.DangerousExplanation)
			}
		} else {
			logger.Debug("Command injected into terminal")
			if selectedCmd.IsDangerous {
				fmt.Fprintf(os.Stderr, "\n⚠️  WARNING: %s\n", selectedCmd.DangerousExplanation)
			}
		}
	}

	historySvc := service.New()
	entry := service.HistoryEntry{
		Timestamp:       time.Now(),
		Prompt:          userPrompt,
		Response:        response,
		Provider:        provider.Name(),
		Model:           provider.Model(),
		SelectedCommand: selectedCmd.Command,
		SelectedIndex:   selectedIndex,
	}
	logger.Debug("Saving to history", "prompt", userPrompt, "provider", provider.Name())
	if err := historySvc.Save(entry); err != nil {
		logger.Warn("Failed to save to history", "error", err)
		fmt.Fprintf(os.Stderr, "Warning: failed to save to history: %v\n", err)
	} else {
		logger.Info("Saved to history")
	}

	return nil
}
