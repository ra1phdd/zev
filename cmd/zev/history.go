package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ra1phdd/zev-go/internal/entities/history/service"
	"github.com/ra1phdd/zev-go/pkg/terminal"
	"github.com/ra1phdd/zev-go/pkg/tui/list"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show query history",
		RunE:  runHistory,
	}
}

func runHistory(cmd *cobra.Command, args []string) error {
	historySvc := service.New()
	entries, err := historySvc.LoadAll()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stdout, "No history found. Run some queries first!")
			return nil
		}
		return fmt.Errorf("failed to load history: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stdout, "No history found. Run some queries first!")
		return nil
	}

	// Сортируем от новых к старым
	reversedEntries := make([]service.HistoryEntry, len(entries))
	for i := range entries {
		reversedEntries[i] = entries[len(entries)-1-i]
	}

	// Формируем список опций для отображения
	items := make([]list.Item, len(reversedEntries))
	for i, entry := range reversedEntries {
		var title string
		switch {
		case entry.SelectedCommand != "":
			title = entry.SelectedCommand
		case entry.Response != nil && len(entry.Response.Commands) > 0:
			title = entry.Response.Commands[0].Command
		default:
			title = "(no commands)"
		}

		description := fmt.Sprintf("%s • %s",
			entry.Timestamp.Format("2006-01-02 15:04"),
			entry.Prompt,
		)

		items[i] = list.NewSimpleItem(title, description)
	}

	selectedItem, err := list.Select(list.SelectOptions{
		Title: "📜 Query History",
		Items: items,
	})
	if err != nil {
		return err
	}

	// Находим индекс выбранного элемента
	var selectedIdx int
	for i := range items {
		if items[i] == selectedItem {
			selectedIdx = i
			break
		}
	}

	// Показываем детальный вид выбранного запроса
	selectedEntry := reversedEntries[selectedIdx]
	return showHistoryDetails(&selectedEntry)
}

func showHistoryDetails(entry *service.HistoryEntry) error {
	if entry.Response == nil || len(entry.Response.Commands) == 0 {
		fmt.Fprintln(os.Stdout, "No commands in this entry")
		return nil
	}

	// Формируем опции для выбора команды
	items := make([]list.Item, len(entry.Response.Commands))
	for i, cmd := range entry.Response.Commands {
		isSelected := cmd.Command == entry.SelectedCommand

		var titlePrefix string
		if isSelected {
			titlePrefix = "✓ "
		}

		var parts []string
		if cmd.ShortExplanation != "" {
			parts = append(parts, cmd.ShortExplanation)
		}
		if cmd.IsDangerous && cmd.DangerousExplanation != "" {
			parts = append(parts, "⚠️  "+cmd.DangerousExplanation)
		}

		items[i] = list.NewSimpleItem(
			titlePrefix+cmd.Command,
			strings.Join(parts, " • "),
		)
	}

	header := fmt.Sprintf("Query: %s\nProvider: %s (%s)\n",
		entry.Prompt,
		entry.Provider,
		entry.Model,
	)

	selectedItem, err := list.Select(list.SelectOptions{
		Title: header,
		Items: items,
	})
	if err != nil {
		return err
	}

	// Находим индекс выбранного элемента
	var selectedIdx int
	for i := range items {
		if items[i] == selectedItem {
			selectedIdx = i
			break
		}
	}

	selectedCommand := entry.Response.Commands[selectedIdx].Command

	// Пытаемся вставить команду в терминал
	if err := terminal.Inject(selectedCommand); err != nil {
		// Если не удалось вставить, просто выводим
		fmt.Fprintf(os.Stdout, "\n✓ Selected command:\n\n  %s\n\n", selectedCommand)
	}

	return nil
}
