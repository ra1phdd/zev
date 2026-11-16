package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/entities/history/repository"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/spf13/cobra"
)

const (
	historyExportJSON   = "json"
	historyExportJSONL  = "jsonl"
	historyStatusFailed = "failed"
)

// historyOperations is the CLI boundary; the repository and search service supply it.
type historyOperations interface {
	GetConversation(context.Context, string) (service.ConversationView, error)
	GetMessage(context.Context, string) (service.Message, error)
	ListConversations(context.Context, service.ListRequest) ([]service.Conversation, error)
	DeleteConversation(context.Context, string) error
	Search(context.Context, service.SearchRequest) (service.SearchResult, error)
	Clear(context.Context) error
}

type historyAdapter struct {
	*repository.Store
	repo     *repository.Store
	embedder service.Embedder
	config   *config.Config
	search   *service.SearchService
}

func (a *historyAdapter) historyStore() *repository.Store {
	if a.repo != nil {
		return a.repo
	}
	return a.Store
}

func (a *historyAdapter) Search(ctx context.Context, req service.SearchRequest) (service.SearchResult, error) {
	if a.search == nil {
		a.search = service.NewSearch(a.historyStore(), a.embedder)
	}
	result, err := a.search.Search(ctx, req)
	if err != nil {
		return service.SearchResult{}, fmt.Errorf("search history service: %w", err)
	}
	return result, nil
}

func (a *historyAdapter) IndexStatus(ctx context.Context, profile string) (service.IndexStatus, error) {
	status, err := a.historyStore().IndexStatus(ctx, profile)
	if err != nil {
		return service.IndexStatus{}, fmt.Errorf("read history index status: %w", err)
	}
	return status, nil
}

func (a *historyAdapter) RebuildFTS(ctx context.Context) error {
	if err := a.historyStore().RebuildFTS(ctx); err != nil {
		return fmt.Errorf("rebuild FTS indexes: %w", err)
	}
	return nil
}

func (a *historyAdapter) RebuildVectors(ctx context.Context, retryFailed bool, limit int) (service.RebuildResult, error) {
	if a.search == nil {
		a.search = service.NewSearch(a.historyStore(), a.embedder)
	}
	result, err := a.search.RebuildVectors(ctx, retryFailed, limit)
	if err != nil {
		return service.RebuildResult{}, fmt.Errorf("rebuild vector service: %w", err)
	}
	return result, nil
}

func (a *historyAdapter) EmbeddingConfig() historyEmbeddingConfig {
	if a.config == nil {
		return historyEmbeddingConfig{}
	}
	return historyEmbeddingConfig{
		Provider: a.config.HistoryEmbeddingProvider,
		Model:    a.config.HistoryEmbeddingModel,
		Enabled:  a.config.HistorySemanticEnabled,
	}
}

func (a *historyAdapter) EmbeddingProfile() string {
	if a.embedder == nil {
		return ""
	}
	return a.embedder.ProfileID()
}

type historyOpener func(context.Context) (historyOperations, io.Closer, error)

func openHistory(ctx context.Context) (historyOperations, io.Closer, error) {
	dir := config.GetZevDir()
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, nil, fmt.Errorf("validate history directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create history directory: %w", err)
	}
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, nil, fmt.Errorf("validate history directory: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load history config: %w", err)
	}
	repo, err := repository.Open(ctx, filepath.Join(dir, "history.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("open history: %w", err)
	}
	embedder, err := newHistoryEmbedder(cfg)
	if errors.Is(err, errHistorySemanticDisabled) {
		embedder = nil
		err = nil
	}
	if err != nil {
		if closeErr := repo.Close(); closeErr != nil {
			return nil, nil, errors.Join(fmt.Errorf("configure history embeddings: %w", err), fmt.Errorf("close history: %w", closeErr))
		}
		return nil, nil, fmt.Errorf("configure history embeddings: %w", err)
	}
	return &historyAdapter{
		Store:    repo,
		repo:     repo,
		embedder: embedder,
		config:   cfg,
		search:   service.NewSearch(repo, embedder),
	}, repo, nil
}

func withHistory(cmd *cobra.Command, open historyOpener, fn func(historyOperations) error) (err error) {
	store, closer, err := open(cmd.Context())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close history: %w", closeErr))
		}
	}()
	return fn(store)
}

func newHistoryCommands(open historyOpener) []*cobra.Command {
	return []*cobra.Command{
		newHistorySearchCmd(open),
		newHistoryShowCmd(open),
		newHistoryExportCmd(open),
		newHistoryDeleteCmd(open),
		newHistoryClearCmd(open),
		newHistoryIndexCmd(open),
	}
}

func newHistoryShowCmd(open historyOpener) *cobra.Command {
	show := &cobra.Command{Use: "show [conversation-id]", Short: "Show a conversation or message", Args: cobra.MaximumNArgs(1)}
	var message string
	var jsonOutput bool
	show.Flags().StringVar(&message, "message", "", "Show a message by full ID")
	show.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON")
	show.RunE = func(cmd *cobra.Command, args []string) error {
		id, showMessage, err := historyShowTarget(args, message)
		if err != nil {
			return err
		}
		_ = jsonOutput // Show remains JSON-compatible by default for scripts.
		return withHistory(cmd, open, func(s historyOperations) error {
			if showMessage {
				v, err := s.GetMessage(cmd.Context(), id)
				if err != nil {
					return fmt.Errorf("get message: %w", err)
				}
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(v); err != nil {
					return fmt.Errorf("encode message: %w", err)
				}
				return nil
			}
			v, err := s.GetConversation(cmd.Context(), id)
			if err != nil {
				return fmt.Errorf("get conversation: %w", err)
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(v); err != nil {
				return fmt.Errorf("encode conversation: %w", err)
			}
			return nil
		})
	}
	return show
}

func historyShowTarget(args []string, message string) (string, bool, error) {
	if (len(args) == 1) == (message != "") {
		return "", false, errors.New("provide exactly one conversation ID or --message ID")
	}
	id := message
	showMessage := message != ""
	if len(args) == 1 {
		id = args[0]
	}
	if strings.TrimSpace(id) == "" {
		return "", false, errors.New("ID must not be empty")
	}
	return id, showMessage, nil
}

func newHistoryExportCmd(open historyOpener) *cobra.Command {
	var format string
	export := &cobra.Command{Use: "export", Short: "Export complete history", Args: cobra.NoArgs}
	export.Flags().StringVar(&format, "format", historyExportJSON, "Output format: json or jsonl")
	export.RunE = func(cmd *cobra.Command, _ []string) error {
		if format != historyExportJSON && format != historyExportJSONL {
			return errors.New("format must be json or jsonl")
		}
		return withHistory(cmd, open, func(s historyOperations) error {
			return streamHistoryExport(cmd.Context(), s, format, cmd.OutOrStdout())
		})
	}
	return export
}

func streamHistoryExport(ctx context.Context, store historyOperations, format string, out io.Writer) error {
	encoder := json.NewEncoder(out)
	if format == historyExportJSON {
		if _, err := io.WriteString(out, "[\n"); err != nil {
			return fmt.Errorf("start JSON export: %w", err)
		}
	}
	first := true
	req := service.ListRequest{Limit: 100}
	for {
		page, err := store.ListConversations(ctx, req)
		if err != nil {
			return fmt.Errorf("list history: %w", err)
		}
		first, err = writeHistoryExportPage(ctx, store, format, out, encoder, page, first)
		if err != nil {
			return err
		}
		if len(page) < req.Limit {
			break
		}
		last := page[len(page)-1]
		if !last.CreatedAt.Before(req.BeforeCreatedAt) && last.ID == req.BeforeID {
			return errors.New("history export cursor did not advance")
		}
		req.BeforeCreatedAt, req.BeforeID = last.CreatedAt, last.ID
	}
	if format == historyExportJSON {
		if _, err := io.WriteString(out, "\n]\n"); err != nil {
			return fmt.Errorf("finish JSON export: %w", err)
		}
	}
	return nil
}

func writeHistoryExportPage(ctx context.Context, store historyOperations, format string, out io.Writer, encoder *json.Encoder, page []service.Conversation, first bool) (bool, error) {
	for _, conversation := range page {
		view, err := store.GetConversation(ctx, conversation.ID)
		if err != nil {
			return first, fmt.Errorf("get conversation: %w", err)
		}
		if format == historyExportJSON && !first {
			if _, err := io.WriteString(out, ",\n"); err != nil {
				return first, fmt.Errorf("separate JSON export: %w", err)
			}
		}
		if err := encoder.Encode(view); err != nil {
			return first, fmt.Errorf("encode history export: %w", err)
		}
		first = false
	}
	return first, nil
}

func newHistoryDeleteCmd(open historyOpener) *cobra.Command {
	return &cobra.Command{Use: "delete <id>", Short: "Delete one conversation", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(args[0]) == "" {
			return errors.New("ID must not be empty")
		}
		return withHistory(cmd, open, func(s historyOperations) error {
			if err := s.DeleteConversation(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("delete conversation: %w", err)
			}
			return nil
		})
	}}
}

func newHistoryClearCmd(open historyOpener) *cobra.Command {
	var yes bool
	clearCmd := &cobra.Command{Use: "clear", Short: "Delete all conversations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !yes {
			return errors.New("clear requires explicit --yes")
		}
		return withHistory(cmd, open, func(s historyOperations) error {
			if err := s.Clear(cmd.Context()); err != nil {
				return fmt.Errorf("clear history: %w", err)
			}
			return nil
		})
	}}
	clearCmd.Flags().BoolVar(&yes, "yes", false, "Confirm deletion of all history")
	return clearCmd
}

func newHistorySearchCmd(open historyOpener) *cobra.Command {
	var req service.SearchRequest
	var from, to string
	var jsonOutput bool
	var dangerous bool
	cmd := &cobra.Command{Use: "search [query]", Short: "Search history", Args: cobra.MaximumNArgs(1)}
	f := cmd.Flags()
	f.StringVar(&req.Mode, "mode", "lexical", "Search mode: lexical, semantic or hybrid")
	f.IntVar(&req.Limit, "limit", 20, "Maximum number of results")
	f.StringVar(&req.Cursor, "cursor", "", "Continue a previous search page")
	f.StringVar(&from, "from", "", "Inclusive UTC date or RFC3339 time")
	f.StringVar(&to, "to", "", "Exclusive UTC date or RFC3339 time")
	f.StringVar(&req.Provider, "provider", "", "Provider filter")
	f.StringVar(&req.Model, "model", "", "Model filter")
	f.StringVar(&req.Status, "status", "", "Conversation status")
	f.StringVar(&req.Type, "type", "", "Document type")
	f.BoolVar(&dangerous, "dangerous", false, "Filter by dangerous command")
	f.StringVar(&req.Fuzzy, "fuzzy", "auto", "Fuzzy mode: auto, always or off")
	f.StringVar(&req.ExactCommand, "exact-command", "", "Exact command text")
	f.BoolVar(&req.Literal, "literal", false, "Search literal text")
	f.BoolVar(&jsonOutput, "json", false, "Output JSON")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			req.Query = args[0]
		}
		if err := validateHistorySearch(&req, from, to, dangerous, f.Changed("dangerous")); err != nil {
			return err
		}
		return withHistory(cmd, open, func(s historyOperations) error {
			result, err := s.Search(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("search history: %w", err)
			}
			return writeHistorySearchResult(cmd, result, jsonOutput)
		})
	}
	return cmd
}

func validateHistorySearch(req *service.SearchRequest, from, to string, dangerous, dangerousChanged bool) error {
	if strings.TrimSpace(req.Query) == "" && req.ExactCommand == "" {
		return errors.New("query must not be empty unless --exact-command is set")
	}
	if !utf8.ValidString(req.Query) || !utf8.ValidString(req.ExactCommand) {
		return errors.New("query must be valid UTF-8")
	}
	if req.Mode != "lexical" && req.Mode != "semantic" && req.Mode != "hybrid" {
		return errors.New("mode must be lexical, semantic or hybrid")
	}
	if req.Fuzzy != "auto" && req.Fuzzy != "always" && req.Fuzzy != "off" {
		return errors.New("fuzzy must be auto, always or off")
	}
	if req.Limit <= 0 {
		return errors.New("limit must be positive")
	}
	if req.Type != "" && req.Type != "user" && req.Type != "assistant" && req.Type != "command" && req.Type != "explanation" {
		return errors.New("type must be user, assistant, command or explanation")
	}
	if req.Status != "" && req.Status != "started" && req.Status != "completed" && req.Status != "cancelled" && req.Status != historyStatusFailed {
		return errors.New("invalid status")
	}
	parsedFrom, err := parseOptionalHistoryTime(from)
	if err != nil {
		return fmt.Errorf("invalid from: %w", err)
	}
	parsedTo, err := parseOptionalHistoryTime(to)
	if err != nil {
		return fmt.Errorf("invalid to: %w", err)
	}
	req.From, req.To = parsedFrom, parsedTo
	if !req.From.IsZero() && !req.To.IsZero() && !req.From.Before(req.To) {
		return errors.New("from must be before to")
	}
	if dangerousChanged {
		req.Dangerous = &dangerous
	}
	return nil
}

func writeHistorySearchResult(cmd *cobra.Command, result service.SearchResult, jsonOutput bool) error {
	if jsonOutput {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
			return fmt.Errorf("encode search result: %w", err)
		}
		return nil
	}
	if result.Warning != "" {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), result.Warning); err != nil {
			return fmt.Errorf("write search warning: %w", err)
		}
	}
	for _, hit := range result.Hits {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%q\t%s\t%q\t%g\t%q\n", hit.ConversationID, hit.CreatedAt.Format(time.RFC3339), hit.DocumentType, hit.Score, hit.Snippet); err != nil {
			return fmt.Errorf("write search result: %w", err)
		}
	}
	return nil
}

func parseOptionalHistoryTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return parseHistoryTime(value)
}

func parseHistoryTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	return t.UTC(), err
}

func safeHistoryPrompt(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' || !utf8.ValidRune(r) || r < 0x20 || r == 0x7f {
			if b.Len() > 0 && !strings.HasSuffix(b.String(), " ") {
				b.WriteByte(' ')
			}
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
