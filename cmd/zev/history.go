package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/pkg/terminal"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show query history",
		RunE:  runHistory,
	}
}

func runHistory(cmd *cobra.Command, _ []string) error {
	historySvc := service.New()
	entries, err := historySvc.LoadAll()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entries = nil
		} else {
			return fmt.Errorf("failed to load history: %w", err)
		}
	}

	if len(entries) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "No history found. Run some queries first!"); err != nil {
			return fmt.Errorf("write empty history: %w", err)
		}
		return nil
	}

	reversedEntries := make([]service.HistoryEntry, len(entries))
	for i := range entries {
		reversedEntries[i] = entries[len(entries)-1-i]
	}

	items := make([]list.Item, len(reversedEntries))
	for i, entry := range reversedEntries {
		if err := validateHistoryEntry(&entry); err != nil {
			return fmt.Errorf("history entry %d: %w", len(entries)-i, err)
		}
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

	selectedItem, err := list.SelectContext(cmd.Context(), list.SelectOptions{
		Title: "📜 Query History",
		Items: items,
	})
	if err != nil {
		return fmt.Errorf("select history entry: %w", err)
	}

	var selectedIdx int
	for i := range items {
		if items[i] == selectedItem {
			selectedIdx = i
			break
		}
	}

	selectedEntry := reversedEntries[selectedIdx]
	return showHistoryDetails(cmd.Context(), &selectedEntry, cmd.OutOrStdout())
}

func validateHistoryEntry(entry *service.HistoryEntry) error {
	if entry.SelectedCommand != "" {
		selected := domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: entry.SelectedCommand}}}
		if err := selected.Validate(); err != nil {
			return fmt.Errorf("invalid selected history command: %w", err)
		}
	}
	// Legacy history can contain rejected requests or no response, but not selectable commands.
	if entry.Response == nil || (!entry.Response.IsValid && len(entry.Response.Commands) == 0) {
		return nil
	}
	if err := entry.Response.Validate(); err != nil {
		return fmt.Errorf("invalid history response: %w", err)
	}
	if !entry.Response.IsValid {
		return fmt.Errorf("%w: rejected history response contains commands", domain.ErrInvalidOptionsResponse)
	}
	return nil
}

func showHistoryDetails(ctx context.Context, entry *service.HistoryEntry, out io.Writer) error {
	if err := validateHistoryEntry(entry); err != nil {
		return err
	}
	if entry.Response == nil || len(entry.Response.Commands) == 0 {
		if _, err := fmt.Fprintln(out, "No commands in this entry"); err != nil {
			return fmt.Errorf("write empty history entry: %w", err)
		}
		return nil
	}

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

	selectedItem, err := list.SelectContext(ctx, list.SelectOptions{
		Title: header,
		Items: items,
	})
	if err != nil {
		return fmt.Errorf("select history command: %w", err)
	}

	var selectedIdx int
	for i := range items {
		if items[i] == selectedItem {
			selectedIdx = i
			break
		}
	}
	selectedCommand := entry.Response.Commands[selectedIdx].Command

	if err := terminal.Inject(selectedCommand); err != nil {
		if _, err := fmt.Fprintf(out, "\n✓ Selected command:\n\n  %s\n\n", selectedCommand); err != nil {
			return fmt.Errorf("write selected history command: %w", err)
		}
	}

	return nil
}
