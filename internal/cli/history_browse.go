package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/spf13/cobra"
)

type historyBrowseEntry struct {
	conversation service.Conversation
}

func historyBrowse(open historyOpener) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		return withHistory(cmd, open, func(s historyOperations) error {
			return runHistoryBrowse(cmd.Context(), cmd, s)
		})
	}
}

func runHistoryBrowse(ctx context.Context, cmd *cobra.Command, store historyOperations) error {
	entries, err := loadHistoryBrowseEntries(ctx, store)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), locale.Text(locale.KeyCliNoHistoryFoundRunSomeQueriesFirst)); err != nil {
			return fmt.Errorf("write empty history message: %w", err)
		}
		return nil
	}
	return selectHistoryBrowseEntry(ctx, cmd, store, entries)
}

func selectHistoryBrowseEntry(ctx context.Context, cmd *cobra.Command, store historyOperations, entries []historyBrowseEntry) error {
	return selectHistoryBrowseEntryWith(ctx, cmd, store, entries, func(options list.SelectOptions) (list.Item, error) {
		return list.SelectContext(ctx, options)
	})
}

func selectHistoryBrowseEntryWith(ctx context.Context, cmd *cobra.Command, store historyOperations, entries []historyBrowseEntry, selectItem func(list.SelectOptions) (list.Item, error)) error {
	items := makeHistoryBrowseItems(entries)
	selected, err := selectItem(list.SelectOptions{Title: locale.Text(locale.KeyCliQueryHistory), Items: items})
	if err != nil {
		if err.Error() == queryCancelled {
			return nil
		}
		return fmt.Errorf(locale.Text(locale.KeyCliSelectHistoryError), err)
	}
	entry, ok := selectedHistoryEntry(items, entries, selected)
	if !ok {
		return errors.New("selected history item not found")
	}
	view, err := store.GetConversation(ctx, entry.conversation.ID)
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliGetConversationError), err)
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(view); err != nil {
		return fmt.Errorf("encode conversation: %w", err)
	}
	return nil
}

func loadHistoryBrowseEntries(ctx context.Context, store historyOperations) ([]historyBrowseEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(locale.Text(locale.KeyCliListHistoryError), err)
	}
	var entries []historyBrowseEntry
	req := service.ListRequest{Limit: 100}
	for {
		page, err := store.ListConversations(ctx, req)
		if err != nil {
			return nil, fmt.Errorf(locale.Text(locale.KeyCliListHistoryError), err)
		}
		for _, conversation := range page {
			entries = append(entries, historyBrowseEntry{conversation: conversation})
		}
		if len(page) < req.Limit {
			return entries, nil
		}
		last := page[len(page)-1]
		if !last.CreatedAt.Before(req.BeforeCreatedAt) && last.ID == req.BeforeID {
			return nil, errors.New("history browse cursor did not advance")
		}
		req.BeforeCreatedAt, req.BeforeID = last.CreatedAt, last.ID
	}
}

func historyBrowsePrompt(conversation service.Conversation) string {
	if prompt := safeHistoryPrompt(conversation.InitialPrompt); prompt != "" {
		return strconv.Quote(prompt)
	}
	return strconv.Quote(fmt.Sprintf(locale.Text(locale.KeyCliConversationMessage), conversation.ID))
}

func makeHistoryBrowseItems(entries []historyBrowseEntry) []list.Item {
	items := make([]list.Item, len(entries))
	for i, entry := range entries {
		items[i] = list.NewSimpleItem(
			historyBrowsePrompt(entry.conversation),
			fmt.Sprintf("%s • %s • %s • %s", entry.conversation.CreatedAt.Format("2006-01-02 15:04"), entry.conversation.Status, entry.conversation.Provider, entry.conversation.ID),
		)
	}
	return items
}

func selectedHistoryEntry(items []list.Item, entries []historyBrowseEntry, selected list.Item) (historyBrowseEntry, bool) {
	for i, item := range items {
		if item == selected {
			return entries[i], true
		}
	}
	return historyBrowseEntry{}, false
}
