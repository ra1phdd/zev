package cli

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
	"github.com/ra1phdd/zev/internal/core/locale"
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
		return service.IndexStatus{}, fmt.Errorf(locale.Text(locale.KeyCliReadHistoryIndexStatusError), err)
	}
	return status, nil
}

func (a *historyAdapter) RebuildFTS(ctx context.Context) error {
	if err := a.historyStore().RebuildFTS(ctx); err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliRebuildFtsIndexesError), err)
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
	dir, err := config.GetZevDir()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve Zev directory: %w", err)
	}
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, nil, fmt.Errorf("validate history directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create history directory: %w", err)
	}
	if err := filestore.ValidateDirectory(dir); err != nil {
		return nil, nil, fmt.Errorf("validate history directory: %w", err)
	}
	cfg, err := loadConfig(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load history config: %w", err)
	}
	repo, err := openHistoryStore(ctx, filepath.Join(dir, "history.db"))
	if err != nil {
		return nil, nil, fmt.Errorf(locale.Text(locale.KeyCliOpenHistoryError), err)
	}
	embedder, err := newHistoryEmbedder(cfg, proxyFromContext(ctx))
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
	store, closer, err := open(contextWithCommandProxy(cmd.Context(), commandProxy(cmd)))
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
	show := &cobra.Command{Use: "show [conversation-id]", Short: locale.Text(locale.KeyCliShowConversationMessage), Args: cobra.MaximumNArgs(1)}
	var message string
	var jsonOutput bool
	show.Flags().StringVar(&message, "message", "", locale.Text(locale.KeyCliShowMessageFullISummary))
	show.Flags().BoolVar(&jsonOutput, "json", false, locale.Text(locale.KeyCliOutputJSON))
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
					return fmt.Errorf(locale.Text(locale.KeyCliGetMessageError), err)
				}
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(v); err != nil {
					return fmt.Errorf("encode message: %w", err)
				}
				return nil
			}
			v, err := s.GetConversation(cmd.Context(), id)
			if err != nil {
				return fmt.Errorf(locale.Text(locale.KeyCliGetConversationError), err)
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
		return "", false, errors.New(locale.Text(locale.KeyCliProvideExactlyOneConversationIDMessageISummary))
	}
	id := message
	showMessage := message != ""
	if len(args) == 1 {
		id = args[0]
	}
	if strings.TrimSpace(id) == "" {
		return "", false, errors.New(locale.Text(locale.KeyCliIDMustNotEmpty))
	}
	return id, showMessage, nil
}

func newHistoryExportCmd(open historyOpener) *cobra.Command {
	var format string
	export := &cobra.Command{Use: "export", Short: locale.Text(locale.KeyCliExportCompleteHistory), Args: cobra.NoArgs}
	export.Flags().StringVar(&format, "format", historyExportJSON, locale.Text(locale.KeyCliOutputFormatJSONJSONL))
	export.RunE = func(cmd *cobra.Command, _ []string) error {
		if format != historyExportJSON && format != historyExportJSONL {
			return errors.New(locale.Text(locale.KeyCliFormatMustJSONJSONL))
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
			return fmt.Errorf(locale.Text(locale.KeyCliListHistoryError), err)
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
			return first, fmt.Errorf(locale.Text(locale.KeyCliGetConversationError), err)
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
	return &cobra.Command{Use: "delete <id>", Short: locale.Text(locale.KeyCliDeleteOneConversation), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(args[0]) == "" {
			return errors.New(locale.Text(locale.KeyCliIDMustNotEmpty))
		}
		return withHistory(cmd, open, func(s historyOperations) error {
			if err := s.DeleteConversation(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf(locale.Text(locale.KeyCliDeleteConversationError), err)
			}
			return nil
		})
	}}
}

func newHistoryClearCmd(open historyOpener) *cobra.Command {
	var yes bool
	clearCmd := &cobra.Command{Use: "clear", Short: locale.Text(locale.KeyCliDeleteAllConversations), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !yes {
			return errors.New(locale.Text(locale.KeyCliClearRequiresExplicitYes))
		}
		return withHistory(cmd, open, func(s historyOperations) error {
			if err := s.Clear(cmd.Context()); err != nil {
				return fmt.Errorf(locale.Text(locale.KeyCliClearHistoryError), err)
			}
			return nil
		})
	}}
	clearCmd.Flags().BoolVar(&yes, "yes", false, locale.Text(locale.KeyCliConfirmDeletionAllHistory))
	return clearCmd
}

func newHistorySearchCmd(open historyOpener) *cobra.Command {
	var req service.SearchRequest
	var from, to string
	var jsonOutput bool
	var dangerous bool
	cmd := &cobra.Command{Use: "search [query]", Short: locale.Text(locale.KeyCliSearchHistory), Args: cobra.MaximumNArgs(1)}
	f := cmd.Flags()
	f.StringVar(&req.Mode, "mode", "lexical", locale.Text(locale.KeyCliSearchModeLexicalSemanticHybrid))
	f.IntVar(&req.Limit, "limit", 20, locale.Text(locale.KeyCliMaximumNumberResults))
	f.StringVar(&req.Cursor, "cursor", "", locale.Text(locale.KeyCliContinuePreviousSearchPage))
	f.StringVar(&from, "from", "", locale.Text(locale.KeyCliInclusiveUtcDateRfc3339Time))
	f.StringVar(&to, "to", "", locale.Text(locale.KeyCliExclusiveUtcDateRfc3339Time))
	f.StringVar(&req.Provider, "provider", "", locale.Text(locale.KeyCliProviderFilter))
	f.StringVar(&req.Model, "model", "", locale.Text(locale.KeyCliModelFilter))
	f.StringVar(&req.Status, "status", "", locale.Text(locale.KeyCliConversationStatus))
	f.StringVar(&req.Type, "type", "", locale.Text(locale.KeyCliDocumentType))
	f.BoolVar(&dangerous, "dangerous", false, locale.Text(locale.KeyCliFilterDangerousCommand))
	f.StringVar(&req.Fuzzy, "fuzzy", "auto", locale.Text(locale.KeyCliFuzzyModeAutoAlwaysOff))
	f.StringVar(&req.ExactCommand, "exact-command", "", locale.Text(locale.KeyCliExactCommandText))
	f.BoolVar(&req.Literal, "literal", false, locale.Text(locale.KeyCliSearchLiteralText))
	f.BoolVar(&jsonOutput, "json", false, locale.Text(locale.KeyCliOutputJSON))
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
				return fmt.Errorf(locale.Text(locale.KeyCliSearchHistoryError), err)
			}
			return writeHistorySearchResult(cmd, result, jsonOutput)
		})
	}
	return cmd
}

const historySearchModeLexical = "lexical"

func validateHistorySearch(req *service.SearchRequest, from, to string, dangerous, dangerousChanged bool) error {
	if err := validateHistorySearchInput(req); err != nil {
		return err
	}
	if err := validateHistorySearchFilters(req); err != nil {
		return err
	}
	if err := setHistorySearchRange(req, from, to); err != nil {
		return err
	}
	if dangerousChanged {
		req.Dangerous = &dangerous
	}
	return nil
}

func validateHistorySearchInput(req *service.SearchRequest) error {
	if strings.TrimSpace(req.Query) == "" && strings.TrimSpace(req.ExactCommand) == "" {
		return errors.New(locale.Text(locale.KeyCliQueryMustNotEmptyUnlessExactCommand))
	}
	if req.ExactCommand != "" && strings.TrimSpace(req.ExactCommand) == "" {
		return errors.New(locale.Text(locale.KeyCliQueryMustNotEmptyUnlessExactCommand))
	}
	if !utf8.ValidString(req.Query) || !utf8.ValidString(req.ExactCommand) {
		return errors.New(locale.Text(locale.KeyCliQueryMustValidUtf8))
	}
	return nil
}

func validateHistorySearchFilters(req *service.SearchRequest) error {
	if req.Limit > 1000 {
		return errors.New(locale.Text(locale.KeyCliLimitMustNotExceed1000))
	}
	if req.Mode != historySearchModeLexical && req.Mode != "semantic" && req.Mode != "hybrid" {
		return errors.New(locale.Text(locale.KeyCliModeMustLexicalSemanticHybrid))
	}
	if req.Fuzzy != toolModeAuto && req.Fuzzy != "always" && req.Fuzzy != "off" {
		return errors.New(locale.Text(locale.KeyCliFuzzyMustAutoAlwaysOff))
	}
	if req.Limit <= 0 {
		return errors.New(locale.Text(locale.KeyCliLimitMustPositive))
	}
	if req.Type != "" && req.Type != "user" && req.Type != "assistant" && req.Type != "command" && req.Type != "explanation" {
		return errors.New(locale.Text(locale.KeyCliTypeMustUserAssistantCommandExplanation))
	}
	if req.Status != "" && req.Status != "started" && req.Status != "completed" && req.Status != "cancelled" && req.Status != historyStatusFailed {
		return errors.New(locale.Text(locale.KeyCliInvalidStatus))
	}
	return nil
}

func setHistorySearchRange(req *service.SearchRequest, from, to string) error {
	parsedFrom, err := parseOptionalHistoryTime(from)
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliInvalidError), err)
	}
	parsedTo, err := parseOptionalHistoryTime(to)
	if err != nil {
		return fmt.Errorf(locale.Text(locale.KeyCliInvalidW2), err)
	}
	req.From, req.To = parsedFrom, parsedTo
	if !req.From.IsZero() && !req.To.IsZero() && !req.From.Before(req.To) {
		return errors.New(locale.Text(locale.KeyCliMustBefore))
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
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), locale.TextSource(result.Warning)); err != nil {
			return fmt.Errorf("write search warning: %w", err)
		}
	}
	for _, hit := range result.Hits {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%q	%s	%q	%g	%q\n", hit.ConversationID, hit.CreatedAt.Format(time.RFC3339), hit.DocumentType, hit.Score, hit.Snippet); err != nil {
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
		if r == '\n' || r == '\r' || r == '	' || !utf8.ValidRune(r) || r < 0x20 || r == 0x7f {
			if b.Len() > 0 && !strings.HasSuffix(b.String(), " ") {
				b.WriteByte(' ')
			}
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
