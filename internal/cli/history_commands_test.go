package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/spf13/cobra"
)

const (
	historyTestFiles            = "files"
	historyTestQuery            = "file"
	historyTestID               = "full-message-id-123456789"
	historyCommandSearch        = "search"
	historyCommandShow          = "show"
	historyCommandClear         = "clear"
	historyCommandIndex         = "index"
	historyCommandRebuild       = "rebuild"
	historyBadValue             = "bad"
	historyFormatFlag           = "--format"
	historyYesFlag              = "--yes"
	historyVectorsFlag          = "--vectors"
	historyJSONFlag             = "--json"
	historyRootCommand          = "history"
	historyBrowseConversationID = "conversation-id"
	historyStatusCompleted      = "completed"
	historyBrowsePromptText     = "посмотреть стату докера одноразово снапшот"
	historyOrderFirstRequest    = "first request"
	historyOrderSecondRequest   = "second request"
	historyRootPrompt           = "enumerate files"
)

type historyCLIFake struct {
	request             service.SearchRequest
	clearCalls          int
	deleted             string
	indexStatus         service.IndexStatus
	rebuildFTSCalls     int
	rebuildVectorsCalls int
	retryFailed         bool
	rebuildResult       service.RebuildResult
	indexErr            error
	ftsErr              error
	vectorsErr          error
	pages               [][]service.Conversation
	listCalls           int
	conversationCalls   int
	openCalls           int
	view                service.ConversationView
	message             service.Message
	listRequests        []service.ListRequest
}

func (f *historyCLIFake) GetConversation(_ context.Context, id string) (service.ConversationView, error) {
	f.conversationCalls++
	if f.view.Conversation.ID == id {
		return f.view, nil
	}
	return service.ConversationView{Conversation: service.Conversation{ID: id}}, nil
}

func (f *historyCLIFake) GetMessage(_ context.Context, id string) (service.Message, error) {
	if f.message.ID == id {
		return f.message, nil
	}
	return service.Message{ID: id}, nil
}

func (f *historyCLIFake) ListConversations(_ context.Context, request service.ListRequest) ([]service.Conversation, error) {
	f.listRequests = append(f.listRequests, request)
	if len(f.pages) == 0 {
		return nil, nil
	}
	page := f.pages[min(f.listCalls, len(f.pages)-1)]
	f.listCalls++
	return page, nil
}

func (f *historyCLIFake) DeleteConversation(_ context.Context, id string) error {
	f.deleted = id
	return nil
}
func (f *historyCLIFake) Clear(context.Context) error { f.clearCalls++; return nil }
func (f *historyCLIFake) Search(_ context.Context, r service.SearchRequest) (service.SearchResult, error) {
	f.request = r
	return service.SearchResult{Hits: []service.SearchHit{{DocumentID: "document-full-id-123456789", ConversationID: "conversation-full-id-123456789", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), DocumentType: historyTestQuery, Snippet: historyTestQuery, Score: 1}}}, nil
}

func (f *historyCLIFake) IndexStatus(context.Context, string) (service.IndexStatus, error) {
	return f.indexStatus, f.indexErr
}

func (f *historyCLIFake) RebuildFTS(context.Context) error {
	f.rebuildFTSCalls++
	return f.ftsErr
}

func (f *historyCLIFake) RebuildVectors(_ context.Context, retryFailed bool, _ int) (service.RebuildResult, error) {
	f.rebuildVectorsCalls++
	f.retryFailed = retryFailed
	return f.rebuildResult, f.vectorsErr
}

func (f *historyCLIFake) EmbeddingConfig() historyEmbeddingConfig {
	return historyEmbeddingConfig{Provider: "openai-compatible", Model: "embedding-test-model", Enabled: true}
}

type historyNopCloser struct{}

func (historyNopCloser) Close() error { return nil }
func historyTestCommand(fake *historyCLIFake, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: historyRootCommand, SilenceErrors: true, SilenceUsage: true}
	cmd.AddCommand(newHistoryCommands(func(context.Context) (historyOperations, io.Closer, error) {
		fake.openCalls++
		return fake, historyNopCloser{}, nil
	})...)
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	return cmd
}

func TestHistorySearchJSONAndFlags(t *testing.T) {
	t.Parallel()
	f := &historyCLIFake{}
	var out bytes.Buffer
	cmd := historyTestCommand(f, &out)
	cmd.SetArgs([]string{historyCommandSearch, historyTestQuery, "--mode", "hybrid", "--fuzzy", "always", "--limit", "4", "--cursor", "next-page", "--from", "2026-01-01", "--to", "2026-02-01", "--provider", "p", "--model", "m", "--status", historyStatusFailed, "--type", queryUserRole, "--dangerous=false", historyJSONFlag})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result service.SearchResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].DocumentID != "document-full-id-123456789" || result.Hits[0].ConversationID != "conversation-full-id-123456789" {
		t.Fatalf("IDs lost: %s", out.String())
	}
	r := f.request
	if r.Query != historyTestQuery || r.Cursor != "next-page" || r.Mode != "hybrid" || r.Fuzzy != "always" || r.Limit != 4 || r.Provider != "p" || r.Model != "m" || r.Status != historyStatusFailed || r.Type != queryUserRole || r.Dangerous == nil || *r.Dangerous || r.From.Format(time.DateOnly) != "2026-01-01" || r.To.Format(time.DateOnly) != "2026-02-01" {
		t.Fatalf("incorrect filters: %+v", r)
	}
}

func TestHistoryExportStreamsPagesWithoutAccumulatingViews(t *testing.T) {
	t.Parallel()
	first := make([]service.Conversation, 100)
	for i := range first {
		first[i] = service.Conversation{ID: fmt.Sprintf("conversation-%03d", i), CreatedAt: time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)}
	}
	fake := &historyCLIFake{pages: [][]service.Conversation{first, {{ID: "conversation-final"}}}}
	var out bytes.Buffer
	if err := streamHistoryExport(t.Context(), fake, historyExportJSON, &out); err != nil {
		t.Fatal(err)
	}
	var views []service.ConversationView
	if err := json.Unmarshal(out.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 101 || fake.listCalls != 2 || fake.conversationCalls != 101 {
		t.Fatalf("export pages=%d views=%d conversations=%d", fake.listCalls, len(views), fake.conversationCalls)
	}
}

func TestHistoryClearRequiresYesBeforeOpening(t *testing.T) {
	t.Parallel()
	f := &historyCLIFake{}
	cmd := historyTestCommand(f, io.Discard)
	cmd.SetArgs([]string{historyCommandClear})
	if err := cmd.Execute(); err == nil || f.clearCalls != 0 || f.openCalls != 0 {
		t.Fatalf("clear without confirmation: %v, clear=%d open=%d", err, f.clearCalls, f.openCalls)
	}
	cmd = historyTestCommand(f, io.Discard)
	cmd.SetArgs([]string{historyCommandClear, historyYesFlag})
	if err := cmd.Execute(); err != nil || f.clearCalls != 1 {
		t.Fatalf("clear with confirmation: %v, %d", err, f.clearCalls)
	}
}

func TestHistoryExactCommandWithoutQuery(t *testing.T) {
	t.Parallel()
	f := &historyCLIFake{}
	cmd := historyTestCommand(f, io.Discard)
	cmd.SetArgs([]string{historyCommandSearch, "--exact-command", "git status --short"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.request.ExactCommand != "git status --short" {
		t.Fatal(f.request)
	}
}

func TestHistoryInteractiveEmptyStore(t *testing.T) {
	t.Parallel()
	f := &historyCLIFake{}
	var out bytes.Buffer
	cmd := historyTestCommand(f, &out)
	cmd.RunE = historyBrowse(func(context.Context) (historyOperations, io.Closer, error) { return f, historyNopCloser{}, nil })
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No history") {
		t.Fatal(out.String())
	}
}

func TestHistoryHasNoDirectSQLImports(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("history*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name == "database/sql" || strings.Contains(name, "sqlite") {
				t.Errorf("%s directly imports %s", path, name)
			}
		}
	}
}

func TestHistoryOpenMigratesLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"timestamp":"2024-03-02T01:02:03Z","prompt":"find archival files","response":{"is_valid":true,"commands":[{"command":"find .","short_explanation":"search files"}]},"provider":"legacy-provider","model":"legacy-model"}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, historyRootCommand), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	opened, closer, err := openHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := opened.(*historyAdapter); !ok {
		t.Fatalf("opened history type = %T", opened)
	}
	if _, err := os.Stat(filepath.Join(dir, "history.db")); err != nil {
		t.Fatalf("history database missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, historyRootCommand)); !os.IsNotExist(err) {
		t.Fatalf("legacy source still active: %v", err)
	}
	archives, err := filepath.Glob(filepath.Join(dir, "history.migrated.*.jsonl"))
	if err != nil || len(archives) != 0 {
		t.Fatalf("legacy archive count = %d, error = %v; migration must delete the source", len(archives), err)
	}
}

func TestHistoryOpenAndSearchRealStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store, closer, err := openHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Errorf("close history: %v", err)
		}
	})
	result, err := store.Search(t.Context(), service.SearchRequest{Query: historyTestQuery, Mode: historySearchModeLexical, Fuzzy: toolModeAuto, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("new store not empty: %+v", result.Hits)
	}
}

func TestHistoryShowMessageID(t *testing.T) {
	t.Parallel()
	f := &historyCLIFake{}
	var out bytes.Buffer
	cmd := historyTestCommand(f, &out)
	cmd.SetArgs([]string{historyCommandShow, "--message", historyTestID, historyJSONFlag})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), historyTestID) {
		t.Fatal(out.String())
	}
}

func TestHistoryShowRendersReadableConversationByDefault(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{view: service.ConversationView{
		Conversation: service.Conversation{ID: historyBrowseConversationID, Status: historyStatusCompleted, InitialPrompt: historyBrowsePromptText},
		Messages:     []service.Message{{Role: queryUserRole, Content: historyBrowsePromptText}, {Role: queryAssistantRole, Content: `{"commands":[{"command":"docker stats --no-stream"}]}`}},
		Responses:    []service.Response{{Commands: []service.Command{{Command: "docker stats --no-stream", ShortExplanation: "Однократный снимок контейнеров"}}}},
	}}
	var out bytes.Buffer
	cmd := historyTestCommand(fake, &out)
	cmd.SetArgs([]string{historyCommandShow, historyBrowseConversationID})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "docker stats --no-stream") || !strings.Contains(out.String(), "посмотреть стату докера") {
		t.Fatalf("readable history show output = %q", out.String())
	}
	if strings.Contains(out.String(), `"Conversation"`) || strings.Contains(out.String(), `{"commands"`) {
		t.Fatalf("history show leaked persistence JSON: %q", out.String())
	}
}

func TestHistoryBrowseUsesReadableConversationLabel(t *testing.T) {
	t.Parallel()
	conversation := service.Conversation{
		ID:            historyTestID,
		InitialPrompt: "find archival files",
		MetadataJSON:  `{"source":"legacy_jsonl","source_line":17}`,
	}
	label := historyBrowsePrompt(conversation)
	if !strings.Contains(label, "find archival files") || strings.Contains(label, "legacy_jsonl") || strings.Contains(label, "source_line") {
		t.Fatalf("history label = %q", label)
	}
}

func TestHistoryBrowseRendersReadableConversationByDefault(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{view: service.ConversationView{
		Conversation: service.Conversation{
			ID:            historyBrowseConversationID,
			Status:        historyStatusCompleted,
			Provider:      "test-provider",
			Model:         "claude-opus-5-5",
			InitialPrompt: historyBrowsePromptText,
		},
		Messages: []service.Message{
			{Role: queryUserRole, Content: historyBrowsePromptText},
			{Role: queryAssistantRole, Content: `{"commands":[{"command":"docker stats --no-stream"}]}`},
		},
		Responses: []service.Response{{Commands: []service.Command{
			{Command: "docker stats --no-stream", ShortExplanation: "Однократный снимок контейнеров", DeliveryOutcome: "injected"},
		}}},
	}}
	entries := []historyBrowseEntry{{conversation: fake.view.Conversation}}
	items := makeHistoryBrowseItems(entries)
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	selectItem := func(list.SelectOptions) (list.Item, error) { return items[0], nil }

	if err := selectHistoryBrowseEntryWith(t.Context(), cmd, fake, entries, selectItem); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "docker stats --no-stream") || !strings.Contains(out.String(), "посмотреть стату докера") {
		t.Fatalf("readable history output = %q", out.String())
	}
	if strings.Contains(out.String(), `"Conversation"`) || strings.Contains(out.String(), `"PayloadJSON"`) || strings.Contains(out.String(), `{"commands"`) {
		t.Fatalf("history output leaked persistence JSON: %q", out.String())
	}
}

func TestHistoryBrowseDoesNotLoadEveryPage(t *testing.T) {
	t.Parallel()
	first := make([]service.Conversation, 100)
	for i := range first {
		first[i] = service.Conversation{ID: fmt.Sprintf("conversation-%03d", i), CreatedAt: time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)}
	}
	fake := &historyCLIFake{pages: [][]service.Conversation{first, {{ID: "conversation-next"}}}}
	page, err := loadHistoryBrowsePage(t.Context(), fake, service.ListRequest{Limit: historyBrowsePageSize + 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.entries) != historyBrowsePageSize || !page.hasNext || fake.listCalls != 1 || len(fake.listRequests) != 1 {
		t.Fatalf("history page entries=%d hasNext=%t list calls=%d requests=%+v", len(page.entries), page.hasNext, fake.listCalls, fake.listRequests)
	}
	if fake.listRequests[0].Limit != historyBrowsePageSize+1 {
		t.Fatalf("history page request = %+v", fake.listRequests[0])
	}
}

func TestHistoryBrowseCanMoveToOlderPage(t *testing.T) {
	t.Parallel()
	first := make([]service.Conversation, historyBrowsePageSize+1)
	for i := range first {
		first[i] = service.Conversation{ID: fmt.Sprintf("conversation-%03d", i), CreatedAt: time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)}
	}
	older := service.Conversation{ID: "conversation-older", InitialPrompt: "older conversation"}
	fake := &historyCLIFake{
		pages: [][]service.Conversation{first, {older}},
		view:  service.ConversationView{Conversation: older},
	}
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	selectionCalls := 0
	selectItem := func(options list.SelectOptions) (list.Item, error) {
		selectionCalls++
		if selectionCalls == 1 {
			return options.Items[len(options.Items)-1], nil
		}
		return options.Items[0], nil
	}
	if err := runHistoryBrowseWith(t.Context(), cmd, fake, false, selectItem); err != nil {
		t.Fatal(err)
	}
	if fake.listCalls != 2 || selectionCalls != 2 || len(fake.listRequests) != 2 || !strings.Contains(out.String(), "older conversation") {
		t.Fatalf("page navigation calls=%d selections=%d requests=%+v output=%q", fake.listCalls, selectionCalls, fake.listRequests, out.String())
	}
	if fake.listRequests[1].Limit != historyBrowsePageSize+1 || fake.listRequests[1].BeforeID != first[historyBrowsePageSize-1].ID || !fake.listRequests[1].BeforeCreatedAt.Equal(first[historyBrowsePageSize-1].CreatedAt) {
		t.Fatalf("older page request = %+v", fake.listRequests[1])
	}
}

func TestHistoryBrowseHumanOutputRemovesTerminalControls(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	view := service.ConversationView{
		Conversation: service.Conversation{ID: "conversation\x1b[id", Status: "status\x1b[status", Provider: "provider\x1b[provider", Model: "model\x1b[model", InitialPrompt: "safe prompt"},
		Messages:     []service.Message{{Role: queryUserRole, Content: "before\x1b[31mred\x1b[0mafter"}},
		Responses:    []service.Response{{Commands: []service.Command{{Command: "printf '\x1b[31mred'", ShortExplanation: "explanation\x1b[0m"}}}},
	}
	if err := writeHistoryConversation(&out, view, false); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(out.String(), '\x1b') {
		t.Fatalf("human history output contains terminal control: %q", out.String())
	}
}

func TestHistoryMessageHumanOutputRemovesTerminalControls(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	message := service.Message{ID: "message\x1b[id", Role: "user\x1b[role", Content: "safe content"}
	if err := writeHistoryMessage(&out, message, false); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(out.String(), '\x1b') {
		t.Fatalf("human message output contains terminal control: %q", out.String())
	}
}

func TestHistoryBrowseJSONRemainsExplicitMachineOutput(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{view: service.ConversationView{
		Conversation: service.Conversation{ID: historyBrowseConversationID, Status: historyStatusCompleted},
	}}
	entries := []historyBrowseEntry{{conversation: fake.view.Conversation}}
	items := makeHistoryBrowseItems(entries)
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	selectItem := func(list.SelectOptions) (list.Item, error) { return items[0], nil }

	if err := selectHistoryBrowseEntryWithJSON(t.Context(), cmd, fake, entries, true, selectItem); err != nil {
		t.Fatal(err)
	}
	var view service.ConversationView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("JSON output = %q: %v", out.String(), err)
	}
	if view.Conversation.ID != historyBrowseConversationID {
		t.Fatalf("conversation ID = %q", view.Conversation.ID)
	}
}

func TestHistoryBrowseCancellationMatchesQueryInteractiveMode(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{}
	entries := []historyBrowseEntry{{conversation: service.Conversation{ID: historyTestID}}}
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	selectItem := func(list.SelectOptions) (list.Item, error) {
		return nil, errors.New(queryCancelled)
	}
	if err := selectHistoryBrowseEntryWith(cmd.Context(), cmd, fake, entries, selectItem); err != nil {
		t.Fatalf("cancelled history selection = %v, want nil", err)
	}
}

func TestHistoryBrowseJSONListsSummariesWithoutSelection(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{pages: [][]service.Conversation{{
		{ID: "conversation-1", Status: historyStatusCompleted, Provider: openAIKind, Model: "model-a", InitialPrompt: "find files"},
	}}}
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	selectItem := func(list.SelectOptions) (list.Item, error) {
		return nil, errors.New("JSON history must not open a selector")
	}

	if err := runHistoryBrowseWith(t.Context(), cmd, fake, true, selectItem); err != nil {
		t.Fatal(err)
	}
	var summaries []map[string]any
	if err := json.Unmarshal(out.Bytes(), &summaries); err != nil {
		t.Fatalf("JSON history output = %q: %v", out.String(), err)
	}
	if len(summaries) != 1 || summaries[0]["id"] != "conversation-1" || summaries[0]["initialPrompt"] != "find files" {
		t.Fatalf("JSON history summaries = %#v", summaries)
	}
}

func TestHistoryReadableOutputPreservesConversationOrder(t *testing.T) {
	t.Parallel()
	view := service.ConversationView{
		Conversation: service.Conversation{ID: "conversation-order", InitialPrompt: historyOrderFirstRequest},
		Messages: []service.Message{
			{Sequence: 0, Role: queryUserRole, Content: historyOrderFirstRequest},
			{Sequence: 1, Role: queryAssistantRole, Content: `{"is_valid":true,"commands":[{"command":"first response"}]}`},
			{Sequence: 2, Role: queryUserRole, Content: historyOrderSecondRequest},
			{Sequence: 3, Role: queryAssistantRole, Content: `{"is_valid":true,"commands":[{"command":"second response"}]}`},
		},
	}
	var out bytes.Buffer
	if err := writeHistoryConversation(&out, view, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, pair := range [][2]string{{historyOrderFirstRequest, "first response"}, {"first response", historyOrderSecondRequest}, {historyOrderSecondRequest, "second response"}} {
		if strings.Index(text, pair[0]) >= strings.Index(text, pair[1]) {
			t.Fatalf("conversation order = %q, want %q before %q", text, pair[0], pair[1])
		}
	}
}

func TestHistoryReadableOutputShowsInvalidResponseExplanation(t *testing.T) {
	t.Parallel()
	view := service.ConversationView{
		Conversation: service.Conversation{ID: "conversation-invalid"},
		Messages:     []service.Message{{Sequence: 0, Role: queryAssistantRole, Content: `{"is_valid":false,"explanation_if_not_valid":"Please clarify the target directory."}`}},
	}
	var out bytes.Buffer
	if err := writeHistoryConversation(&out, view, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Please clarify the target directory.") {
		t.Fatalf("invalid response explanation missing: %q", out.String())
	}
}

func TestHistoryReadableOutputRejectsC1Controls(t *testing.T) {
	t.Parallel()
	view := service.ConversationView{
		Conversation: service.Conversation{ID: "conversation\u009b31m", InitialPrompt: "prompt"},
		Messages:     []service.Message{{Sequence: 0, Role: queryUserRole, Content: "before\u009b31mafter"}},
	}
	var out bytes.Buffer
	if err := writeHistoryConversation(&out, view, false); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(out.String(), '\u009b') {
		t.Fatalf("C1 control reached human output: %q", out.String())
	}
}

func TestHistoryReadableOutputShowsDangerousCommandWarning(t *testing.T) {
	t.Parallel()
	view := service.ConversationView{
		Conversation: service.Conversation{ID: "conversation-dangerous"},
		Responses:    []service.Response{{IsValid: true, Commands: []service.Command{{Command: "rm -rf tmp", IsDangerous: true, DangerousExplanation: "recursive deletion"}}}},
	}
	var out bytes.Buffer
	if err := writeHistoryConversation(&out, view, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "recursive deletion") {
		t.Fatalf("dangerous explanation missing: %q", out.String())
	}
}

func TestHistoryReadableOutputMarksDangerousCommandWithoutExplanation(t *testing.T) {
	t.Parallel()
	view := service.ConversationView{
		Conversation: service.Conversation{ID: "conversation-dangerous-empty"},
		Responses:    []service.Response{{IsValid: true, Commands: []service.Command{{Command: "rm -rf tmp", IsDangerous: true}}}},
	}
	var out bytes.Buffer
	if err := writeHistoryConversation(&out, view, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Dangerous") && !strings.Contains(out.String(), "Опас") {
		t.Fatalf("dangerous marker missing: %q", out.String())
	}
}

func TestHistoryRootJSONIsNonInteractive(t *testing.T) {
	t.Parallel()
	fake := &historyCLIFake{pages: [][]service.Conversation{{
		{ID: "conversation-root", InitialPrompt: historyRootPrompt},
	}}}
	var out bytes.Buffer
	cmd := newHistoryCmdWithOpener(func(context.Context) (historyOperations, io.Closer, error) {
		return fake, historyNopCloser{}, nil
	})
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{historyJSONFlag})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var summaries []historySummary
	if err := json.Unmarshal(out.Bytes(), &summaries); err != nil {
		t.Fatalf("root history JSON = %q: %v", out.String(), err)
	}
	if len(summaries) != 1 || summaries[0].ID != "conversation-root" {
		t.Fatalf("root history summaries = %#v", summaries)
	}
}
