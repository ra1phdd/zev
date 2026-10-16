package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/core/httpclient"
	"github.com/ra1phdd/zev-go/internal/entities/history/service"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev-go/internal/entities/query/service"
	"github.com/ra1phdd/zev-go/pkg/terminal"
	"github.com/ra1phdd/zev-go/pkg/tui/list"
	"github.com/ra1phdd/zev-go/pkg/tui/prompt"
	"github.com/spf13/cobra"
)

func newQueryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "query [query]",
		Short: "Query for command suggestions",
		Args:  cobra.ArbitraryArgs,
		RunE:  runQuery,
	}
}

func runQuery(cmd *cobra.Command, args []string) error {
	// Загрузка конфигурации
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w. Run `zev setup` to configure", err)
	}

	// Определяем промпт
	var userPrompt string
	if len(args) > 0 {
		userPrompt = strings.Join(args, " ")
	} else {
		// Интерактивный режим
		userPrompt, err = prompt.Input("What command do you need?")
		if err != nil {
			// Если пользователь отменил ввод — это штатное поведение
			if err.Error() == "cancelled" {
				return nil
			}
			return fmt.Errorf("failed to get input: %w", err)
		}
	}

	if strings.TrimSpace(userPrompt) == "" {
		return errors.New("prompt cannot be empty")
	}

	// Создание провайдера
	httpClient := httpclient.New(cfg.HTTPHeaders)
	provider, err := factory.NewProvider(cfg, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	querySvc := queryservice.New(provider)
	ctx := context.Background()

	// Инициализация истории диалога
	history := &domain.ConversationHistory{
		SystemPrompt: repository.SystemPrompt,
		Messages:     []domain.Message{},
	}

	// Первый запрос
	envContext := querySvc.GetEnvContext()
	firstUserMessage := fmt.Sprintf(repository.UserPromptTemplate, envContext, userPrompt)
	history.Messages = append(history.Messages, domain.Message{
		Role:    "user",
		Content: firstUserMessage,
	})

	fmt.Fprintln(os.Stdout, "Fetching suggestions...")
	response, err := provider.GetOptionsWithHistory(ctx, history)
	if err != nil {
		return err
	}

	// Проверка валидности ответа
	if !response.IsValid {
		fmt.Fprintf(os.Stderr, "\n⚠️  Unable to process your request:\n%s\n", response.ExplanationIfNotValid)
		return nil
	}

	if len(response.Commands) == 0 {
		fmt.Fprintln(os.Stdout, "\n⚠️  No commands found for your query.")
		return nil
	}

	// Сохраняем ответ в истории (сокращённый JSON)
	history.Messages = append(history.Messages, domain.Message{
		Role:    "assistant",
		Content: response.CompactJSON(),
	})

	// Добавляем опцию "Уточнить запрос" в конец списка
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

	// Цикл выбора команды (с возможностью уточнения)
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
			// Если пользователь отменил выбор (Ctrl+C, Esc, q) — это штатное поведение
			if err.Error() == "cancelled" {
				return nil
			}
			return fmt.Errorf("failed to select command: %w", err)
		}

		cmdItem := selected.(list.CommandItem)
		selectedCmd = cmdItem.Cmd

		// Если выбрана опция "Уточнить запрос"
		if selectedCmd.Command == refineMarker {
			refinedPrompt, err := prompt.Input("Specify your request:")
			if err != nil {
				// Если пользователь отменил — это штатное поведение
				if err.Error() == "cancelled" {
					return nil
				}
				return fmt.Errorf("failed to get refined input: %w", err)
			}

			if strings.TrimSpace(refinedPrompt) == "" {
				fmt.Fprintln(os.Stderr, "The request cannot be empty.")
				continue
			}

			// Обновляем userPrompt для финальной истории
			userPrompt = refinedPrompt

			// Добавляем уточнение в историю
			history.Messages = append(history.Messages, domain.Message{
				Role:    "user",
				Content: refinedPrompt,
			})

			// Получаем новые варианты команд
			fmt.Fprintln(os.Stdout, "Fetching suggestions...")
			response, err = provider.GetOptionsWithHistory(ctx, history)
			if err != nil {
				return err
			}

			// Проверка валидности ответа
			if !response.IsValid {
				fmt.Fprintf(os.Stderr, "\n⚠️  Unable to process your request:\n%s\n", response.ExplanationIfNotValid)
				return nil
			}

			if len(response.Commands) == 0 {
				fmt.Fprintln(os.Stdout, "\n⚠️  No commands found for your query.")
				return nil
			}

			// Сохраняем ответ в истории (сокращённый JSON)
			history.Messages = append(history.Messages, domain.Message{
				Role:    "assistant",
				Content: response.CompactJSON(),
			})

			// Обновляем список команд для следующей итерации
			commandsWithRefine = make([]domain.Command, len(response.Commands), len(response.Commands)+1)
			copy(commandsWithRefine, response.Commands)
			commandsWithRefine = append(commandsWithRefine, domain.Command{
				Command:          refineMarker,
				ShortExplanation: "Clarify the request",
				IsDangerous:      false,
			})

			continue
		}

		// Обычная команда выбрана — находим её индекс и выходим из цикла
		for i := range response.Commands {
			if response.Commands[i].Command == selectedCmd.Command {
				selectedIndex = i
				break
			}
		}
		break
	}

	// Пытаемся вставить команду в терминал
	if err := terminal.Inject(selectedCmd.Command); err != nil {
		// Если не удалось вставить (не TTY, unsupported platform, или ioctl failed),
		// просто выводим команду
		fmt.Fprintf(os.Stdout, "\n✓ Selected command:\n\n  %s\n\n", selectedCmd.Command)
		if selectedCmd.IsDangerous {
			fmt.Fprintf(os.Stdout, "⚠️  WARNING: %s\n\n", selectedCmd.DangerousExplanation)
		}
		// Не возвращаем ошибку — это не критично, команда всё равно выведена
	} else if selectedCmd.IsDangerous {
		// Команда успешно вставлена — показываем предупреждение если нужно
		fmt.Fprintf(os.Stderr, "\n⚠️  WARNING: %s\n", selectedCmd.DangerousExplanation)
	}

	// Сохранение в историю
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
	if err := historySvc.Save(entry); err != nil {
		// Не останавливаем выполнение из-за ошибки истории
		fmt.Fprintf(os.Stderr, "Warning: failed to save to history: %v\n", err)
	}

	return nil
}
