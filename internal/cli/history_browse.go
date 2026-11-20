package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/spf13/cobra"
)

type historyBrowseEntry struct {
	conversation service.Conversation
}

const historyBrowsePageSize = 20

type historyBrowsePage struct {
	entries []historyBrowseEntry
	next    service.ListRequest
	hasNext bool
}

type historyBrowseNextPageItem struct{}

func (historyBrowseNextPageItem) Title() string {
	return locale.Text(locale.KeyCliHistoryNextPage)
}

func (historyBrowseNextPageItem) Description() string { return "" }
func (historyBrowseNextPageItem) IsWarning() bool     { return false }

func historyBrowse(open historyOpener) func(*cobra.Command, []string) error {
	return historyBrowseWithJSON(open, false)
}

func historyBrowseWithJSON(open historyOpener, jsonOutput bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		return withHistory(cmd, open, func(s historyOperations) error {
			return runHistoryBrowse(cmd.Context(), cmd, s, jsonOutput)
		})
	}
}

func runHistoryBrowse(ctx context.Context, cmd *cobra.Command, store historyOperations, jsonOutput bool) error {
	return runHistoryBrowseWith(ctx, cmd, store, jsonOutput, func(options list.SelectOptions) (list.Item, error) {
		options.Input = cmd.InOrStdin()
		return list.SelectContext(ctx, options)
	})
}

func runHistoryBrowseWith(ctx context.Context, cmd *cobra.Command, store historyOperations, jsonOutput bool, selectItem func(list.SelectOptions) (list.Item, error)) error {
	if jsonOutput {
		return writeHistorySummariesJSON(ctx, store, cmd.OutOrStdout())
	}
	req := service.ListRequest{Limit: historyBrowsePageSize + 1}
	for {
		page, err := loadHistoryBrowsePage(ctx, store, req)
		if err != nil {
			return err
		}
		if len(page.entries) == 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), locale.Text(locale.KeyCliNoHistoryFoundRunSomeQueriesFirst)); err != nil {
				return fmt.Errorf("write empty history message: %w", err)
			}
			return nil
		}
		next, err := selectHistoryBrowsePageWith(ctx, cmd, store, page, jsonOutput, selectItem)
		if err != nil {
			return err
		}
		if !next {
			return nil
		}
		if page.next.BeforeCreatedAt.Equal(req.BeforeCreatedAt) && page.next.BeforeID == req.BeforeID {
			return errors.New("history browse cursor did not advance")
		}
		req = page.next
	}
}

func selectHistoryBrowseEntryWith(ctx context.Context, cmd *cobra.Command, store historyOperations, entries []historyBrowseEntry, selectItem func(list.SelectOptions) (list.Item, error)) error {
	return selectHistoryBrowseEntryWithJSON(ctx, cmd, store, entries, false, selectItem)
}

func selectHistoryBrowseEntryWithJSON(ctx context.Context, cmd *cobra.Command, store historyOperations, entries []historyBrowseEntry, jsonOutput bool, selectItem func(list.SelectOptions) (list.Item, error)) error {
	_, err := selectHistoryBrowsePageWith(ctx, cmd, store, historyBrowsePage{entries: entries}, jsonOutput, selectItem)
	return err
}

func selectHistoryBrowsePageWith(ctx context.Context, cmd *cobra.Command, store historyOperations, page historyBrowsePage, jsonOutput bool, selectItem func(list.SelectOptions) (list.Item, error)) (bool, error) {
	items := makeHistoryBrowseItems(page.entries)
	if page.hasNext {
		items = append(items, historyBrowseNextPageItem{})
	}
	selected, err := selectItem(list.SelectOptions{Title: locale.Text(locale.KeyCliQueryHistory), Items: items})
	if err != nil {
		if err.Error() == queryCancelled {
			return false, nil
		}
		return false, fmt.Errorf(locale.Text(locale.KeyCliSelectHistoryError), err)
	}
	if _, ok := selected.(historyBrowseNextPageItem); ok {
		return true, nil
	}
	entry, ok := selectedHistoryEntry(items, page.entries, selected)
	if !ok {
		return false, errors.New("selected history item not found")
	}
	view, err := store.GetConversation(ctx, entry.conversation.ID)
	if err != nil {
		return false, fmt.Errorf(locale.Text(locale.KeyCliGetConversationError), err)
	}
	return false, writeHistoryConversation(cmd.OutOrStdout(), view, jsonOutput)
}

func writeHistoryConversation(out io.Writer, view service.ConversationView, jsonOutput bool) error {
	if jsonOutput {
		if err := json.NewEncoder(out).Encode(view); err != nil {
			return fmt.Errorf("encode conversation: %w", err)
		}
		return nil
	}
	conversation := view.Conversation
	title := safeHistoryField(conversation.InitialPrompt)
	if title == "" {
		title = fmt.Sprintf(locale.Text(locale.KeyCliConversationMessage), safeHistoryField(conversation.ID))
	}
	if _, err := fmt.Fprintf(out, "%s\n", title); err != nil {
		return fmt.Errorf("write conversation title: %w", err)
	}
	if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliHistoryConversationMeta), safeHistoryField(conversation.ID), safeHistoryField(conversation.Status), safeHistoryField(conversation.Provider), safeHistoryField(conversation.Model), conversation.CreatedAt.Format("2006-01-02 15:04:05"), conversation.UpdatedAt.Format("2006-01-02 15:04:05")); err != nil {
		return fmt.Errorf("write conversation metadata: %w", err)
	}
	if err := writeHistoryMessages(out, view.Messages, view.Responses); err != nil {
		return err
	}
	return nil
}

func writeHistoryMessage(out io.Writer, message service.Message, jsonOutput bool) error {
	if jsonOutput {
		if err := json.NewEncoder(out).Encode(message); err != nil {
			return fmt.Errorf("encode message: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliHistoryMessage), safeHistoryField(message.ID)); err != nil {
		return fmt.Errorf("write message header: %w", err)
	}
	if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliHistoryRole), historyRoleText(message.Role)); err != nil {
		return fmt.Errorf("write message role: %w", err)
	}
	if message.Role == queryAssistantRole {
		return writeAssistantHistoryMessage(out, message.Content)
	}
	if _, err := fmt.Fprintf(out, "\n%s\n", safeHistoryOutputText(message.Content)); err != nil {
		return fmt.Errorf("write message content: %w", err)
	}
	return nil
}

func writeAssistantHistoryMessage(out io.Writer, content string) error {
	var response domain.OptionsResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return writeHistoryMessageContent(out, content)
	}
	commands := make([]service.Command, len(response.Commands))
	for i, command := range response.Commands {
		commands[i] = service.Command{Command: command.Command, ShortExplanation: command.ShortExplanation, DangerousExplanation: command.DangerousExplanation, IsDangerous: command.IsDangerous}
	}
	if !response.IsValid {
		if _, err := fmt.Fprintf(out, "\n%s\n", safeHistoryOutputText(response.ExplanationIfNotValid)); err != nil {
			return fmt.Errorf("write invalid response explanation: %w", err)
		}
		return nil
	}
	return writeHistoryResponses(out, []service.Response{{Commands: commands}})
}

func writeHistoryMessageContent(out io.Writer, content string) error {
	if _, err := fmt.Fprintf(out, "\n%s\n", safeHistoryOutputText(content)); err != nil {
		return fmt.Errorf("write message content: %w", err)
	}
	return nil
}

func writeHistoryMessages(out io.Writer, messages []service.Message, responses []service.Response) error {
	responsesByMessage := make(map[string]service.Response, len(responses))
	for _, response := range responses {
		if response.MessageID != "" {
			responsesByMessage[response.MessageID] = response
		}
	}
	renderedResponses := make(map[string]struct{}, len(responsesByMessage))
	for _, message := range messages {
		rendered, err := writeHistoryMessageEvent(out, message, responsesByMessage)
		if err != nil {
			return err
		}
		if rendered != "" {
			renderedResponses[rendered] = struct{}{}
		}
	}
	for _, response := range responses {
		if _, rendered := renderedResponses[response.ID]; rendered {
			continue
		}
		if err := writeHistoryResponse(out, response); err != nil {
			return err
		}
	}
	return nil
}

func writeHistoryMessageEvent(out io.Writer, message service.Message, responses map[string]service.Response) (string, error) {
	if message.Role == queryAssistantRole {
		return writeHistoryAssistantEvent(out, message, responses)
	}
	if _, err := fmt.Fprintf(out, "\n%s\n%s\n", historyRoleText(message.Role), safeHistoryOutputText(message.Content)); err != nil {
		return "", fmt.Errorf("write conversation message: %w", err)
	}
	return "", nil
}

func writeHistoryAssistantEvent(out io.Writer, message service.Message, responses map[string]service.Response) (string, error) {
	if response, ok := responses[message.ID]; ok {
		if err := writeHistoryResponse(out, response); err != nil {
			return "", err
		}
		return response.ID, nil
	}
	var payload domain.OptionsResponse
	if err := json.Unmarshal([]byte(message.Content), &payload); err != nil {
		return "", writeHistoryMessageContent(out, message.Content)
	}
	if !payload.IsValid && payload.ExplanationIfNotValid == "" {
		return "", nil
	}
	if err := writeHistoryMessage(out, message, false); err != nil {
		return "", err
	}
	return "", nil
}

func writeHistoryResponse(out io.Writer, response service.Response) error {
	if !response.IsValid && len(response.Commands) == 0 {
		var payload domain.OptionsResponse
		if err := json.Unmarshal([]byte(response.PayloadJSON), &payload); err == nil && payload.ExplanationIfNotValid != "" {
			if _, writeErr := fmt.Fprintf(out, "\n%s\n", safeHistoryOutputText(payload.ExplanationIfNotValid)); writeErr != nil {
				return fmt.Errorf("write invalid response explanation: %w", writeErr)
			}
			return nil
		}
		if _, err := fmt.Fprintf(out, "\n%s\n", locale.Text(locale.KeyCliHistoryInvalidResponse)); err != nil {
			return fmt.Errorf("write invalid response marker: %w", err)
		}
		return nil
	}
	return writeHistoryResponses(out, []service.Response{response})
}

func writeHistoryResponses(out io.Writer, responses []service.Response) error {
	if len(responses) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(out, "\n%s\n", locale.Text(locale.KeyCliHistorySuggestions)); err != nil {
		return fmt.Errorf("write conversation suggestions header: %w", err)
	}
	ordinal := 1
	for _, response := range responses {
		for _, command := range response.Commands {
			if err := writeHistoryCommand(out, command, ordinal); err != nil {
				return err
			}
			ordinal++
		}
	}
	return nil
}

func writeHistoryCommand(out io.Writer, command service.Command, ordinal int) error {
	if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliHistoryCommandEntry), ordinal, safeHistoryOutputText(command.Command), safeHistoryOutputText(command.ShortExplanation)); err != nil {
		return fmt.Errorf("write conversation command: %w", err)
	}
	if command.DeliveryOutcome != "" {
		if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliHistoryCommandDelivery), safeHistoryOutputText(command.DeliveryOutcome)); err != nil {
			return fmt.Errorf("write conversation command status: %w", err)
		}
	}
	if command.IsDangerous {
		if command.DangerousExplanation != "" {
			if _, err := fmt.Fprintf(out, locale.Text(locale.KeyCliHistoryCommandWarning), safeHistoryOutputText(command.DangerousExplanation)); err != nil {
				return fmt.Errorf("write conversation command warning: %w", err)
			}
		} else if _, err := fmt.Fprintf(out, "     %s\n", locale.Text(locale.KeyCliHistoryDangerousCommand)); err != nil {
			return fmt.Errorf("write conversation dangerous marker: %w", err)
		}
	}
	return nil
}

func safeHistoryOutputText(text string) string {
	var b strings.Builder
	for _, r := range text {
		if (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) && r != '\n' && r != '	' {
			fmt.Fprintf(&b, "\\u%04x", r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func safeHistoryField(text string) string {
	return safeHistoryPrompt(safeHistoryOutputText(text))
}

func historyRoleText(role string) string {
	switch role {
	case queryUserRole:
		return locale.Text(locale.KeyCliHistoryRoleUser)
	case queryAssistantRole:
		return locale.Text(locale.KeyCliHistoryRoleAssistant)
	default:
		return strings.ToUpper(safeHistoryField(role))
	}
}

type historySummary struct {
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Status        string    `json:"status"`
	Kind          string    `json:"kind"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	InitialPrompt string    `json:"initialPrompt"`
}

func writeHistorySummariesJSON(ctx context.Context, store historyOperations, out io.Writer) error {
	encoder := json.NewEncoder(out)
	if _, err := io.WriteString(out, "[\n"); err != nil {
		return fmt.Errorf("start history summary JSON: %w", err)
	}
	first := true
	req := service.ListRequest{Limit: 100}
	for {
		page, err := store.ListConversations(ctx, req)
		if err != nil {
			return fmt.Errorf(locale.Text(locale.KeyCliListHistoryError), err)
		}
		first, err = writeHistorySummaryPage(out, encoder, page, first)
		if err != nil {
			return err
		}
		if len(page) < req.Limit {
			break
		}
		last := page[len(page)-1]
		if !last.CreatedAt.Before(req.BeforeCreatedAt) && last.ID == req.BeforeID {
			return errors.New("history summary cursor did not advance")
		}
		req.BeforeCreatedAt, req.BeforeID = last.CreatedAt, last.ID
	}
	if _, err := io.WriteString(out, "\n]\n"); err != nil {
		return fmt.Errorf("finish history summary JSON: %w", err)
	}
	return nil
}

func writeHistorySummaryPage(out io.Writer, encoder *json.Encoder, page []service.Conversation, first bool) (bool, error) {
	for _, conversation := range page {
		if !first {
			if _, err := io.WriteString(out, ",\n"); err != nil {
				return first, fmt.Errorf("separate history summary JSON: %w", err)
			}
		}
		first = false
		summary := historySummary{
			ID: conversation.ID, CreatedAt: conversation.CreatedAt, UpdatedAt: conversation.UpdatedAt,
			Status: conversation.Status, Kind: conversation.Kind,
			Provider: conversation.Provider, Model: conversation.Model,
			InitialPrompt: conversation.InitialPrompt,
		}
		if err := encoder.Encode(summary); err != nil {
			return first, fmt.Errorf("encode history summary: %w", err)
		}
	}
	return first, nil
}

func loadHistoryBrowsePage(ctx context.Context, store historyOperations, req service.ListRequest) (historyBrowsePage, error) {
	if err := ctx.Err(); err != nil {
		return historyBrowsePage{}, fmt.Errorf(locale.Text(locale.KeyCliListHistoryError), err)
	}
	if req.Limit <= 0 {
		req.Limit = historyBrowsePageSize + 1
	}
	conversations, err := store.ListConversations(ctx, req)
	if err != nil {
		return historyBrowsePage{}, fmt.Errorf(locale.Text(locale.KeyCliListHistoryError), err)
	}
	visible := conversations
	if len(visible) > historyBrowsePageSize {
		visible = visible[:historyBrowsePageSize]
	}
	entries := make([]historyBrowseEntry, len(visible))
	for i, conversation := range visible {
		entries[i] = historyBrowseEntry{conversation: conversation}
	}
	result := historyBrowsePage{entries: entries}
	if len(conversations) > historyBrowsePageSize {
		last := visible[len(visible)-1]
		result.hasNext = true
		result.next = service.ListRequest{
			Limit:           historyBrowsePageSize + 1,
			BeforeCreatedAt: last.CreatedAt,
			BeforeID:        last.ID,
		}
	}
	return result, nil
}

func historyBrowsePrompt(conversation service.Conversation) string {
	if prompt := safeHistoryField(conversation.InitialPrompt); prompt != "" {
		return strconv.Quote(prompt)
	}
	return strconv.Quote(fmt.Sprintf(locale.Text(locale.KeyCliConversationMessage), safeHistoryField(conversation.ID)))
}

func makeHistoryBrowseItems(entries []historyBrowseEntry) []list.Item {
	items := make([]list.Item, len(entries))
	for i, entry := range entries {
		items[i] = list.NewSimpleItem(
			historyBrowsePrompt(entry.conversation),
			fmt.Sprintf("%s • %s • %s • %s", entry.conversation.CreatedAt.Format("2006-01-02 15:04"), safeHistoryField(entry.conversation.Status), safeHistoryField(entry.conversation.Provider), safeHistoryField(entry.conversation.ID)),
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
