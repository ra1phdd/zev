package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/history/repository"
	"github.com/ra1phdd/zev/internal/entities/history/service"
	"github.com/spf13/cobra"
)

const (
	historyValidationConversationID = "conversation-1"
	historyValidationInitialKind    = "initial_prompt"
	historyValidationPayload        = `{"isValid":true}`
)

type unsafeHistoryFake struct {
	view          service.ConversationView
	conversations []service.Conversation
	getCalls      int
	messageCalls  int
}

func (f *unsafeHistoryFake) GetConversation(context.Context, string) (service.ConversationView, error) {
	f.getCalls++
	return f.view, nil
}

func (f *unsafeHistoryFake) GetMessage(context.Context, string) (service.Message, error) {
	f.messageCalls++
	return service.Message{}, nil
}

func (f *unsafeHistoryFake) ListConversations(context.Context, service.ListRequest) ([]service.Conversation, error) {
	return f.conversations, nil
}
func (f *unsafeHistoryFake) DeleteConversation(context.Context, string) error { return nil }
func (f *unsafeHistoryFake) Search(context.Context, service.SearchRequest) (service.SearchResult, error) {
	return service.SearchResult{}, nil
}
func (f *unsafeHistoryFake) Clear(context.Context) error { return nil }

type unsafeHistoryCloser struct{}

func (unsafeHistoryCloser) Close() error { return nil }

func historyTestCommandFromOpener(fake historyOperations, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: historyRootCommand, SilenceErrors: true, SilenceUsage: true}
	cmd.AddCommand(newHistoryCommands(func(context.Context) (historyOperations, io.Closer, error) {
		return fake, unsafeHistoryCloser{}, nil
	})...)
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	return cmd
}

func TestHistoryShowEscapesStoredControlsWithoutExecutingCommands(t *testing.T) {
	t.Parallel()
	unsafeText := "echo first\x1b[2J\necho second"
	store, err := repository.Open(t.Context(), filepath.Join(t.TempDir(), "private", "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close history store: %v", err)
		}
	})
	if _, err := store.StartConversation(t.Context(), service.StartConversationInput{
		Conversation:   service.Conversation{ID: historyValidationConversationID},
		InitialMessage: service.Message{ID: "message-1", Role: "user", Kind: historyValidationInitialKind, Content: unsafeText},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordResponse(t.Context(), service.RecordResponseInput{
		Message: service.Message{ID: "message-2", Role: queryAssistantRole, Kind: "response", Content: historyValidationPayload},
		Response: service.Response{
			ID: "response-1", ConversationID: historyValidationConversationID, PayloadJSON: historyValidationPayload,
			IsValid: true, Commands: []service.Command{{ID: "command-1", Command: unsafeText}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := historyTestCommandFromOpener(&historyAdapter{Store: store, search: service.NewSearch(store, nil)}, &out)
	cmd.SetArgs([]string{"show", historyValidationConversationID})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), unsafeText) {
		t.Fatalf("stored control text was emitted raw: %q", out.String())
	}
	if !strings.Contains(out.String(), `\u001b`) {
		t.Fatalf("stored control text was not JSON-escaped: %q", out.String())
	}
}

func TestHistoryBrowserDoesNotOpenStoredDetailsBeforeSelection(t *testing.T) {
	t.Parallel()
	fake := &unsafeHistoryFake{conversations: []service.Conversation{{ID: historyValidationConversationID}}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{Use: historyRootCommand, SilenceErrors: true, SilenceUsage: true}
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	opener := func(context.Context) (historyOperations, io.Closer, error) {
		return fake, unsafeHistoryCloser{}, nil
	}
	cmd.RunE = historyBrowse(opener)
	if err := cmd.Execute(); err == nil {
		t.Fatal("cancelled browser unexpectedly succeeded")
	}
	if fake.getCalls != 0 {
		t.Fatalf("browser fetched details before selection: %d calls", fake.getCalls)
	}
}
