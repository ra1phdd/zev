package service

import (
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	contextFileContents        = "README contents"
	contextErrorSection        = "[COMMAND_ERROR]"
	contextExitError           = "exit status 1"
	contextCommandSection      = "[COMMAND]"
	contextConversationSection = "[CONVERSATION]"
)

func TestProviderHistoryFormatsContinuationSections(t *testing.T) {
	t.Parallel()
	history := &domain.ConversationHistory{SystemPrompt: "environment", Messages: []domain.Message{
		{Role: contextUserRole, Kind: "project_context", Content: contextFileContents},
		{Role: contextUserRole, Kind: "command_error", Content: contextExitError},
		{Role: contextUserRole, Kind: "refinement", Content: "fix the failed command"},
		{Role: contextAssistantRole, Kind: "response", Content: `{"commands":["echo fixed"]}`},
	}}
	got := providerHistory(history)
	if got == history {
		t.Fatal("continuation history was not copied")
	}
	want := []string{"[PROJECT_CONTEXT]", contextFileContents, contextErrorSection, contextExitError, contextConversationSection, "fix the failed command", contextCommandSection, "echo fixed"}
	joined := got.Messages[0].Content + got.Messages[1].Content + got.Messages[2].Content + got.Messages[3].Content
	for _, value := range want {
		if !strings.Contains(joined, value) {
			t.Errorf("formatted continuation history %q does not contain %q", joined, value)
		}
	}
	if history.Messages[0].Content != contextFileContents {
		t.Fatal("provider formatting mutated continuation history")
	}
}

func TestProviderHistoryFormatsTypedTurnsWithoutFabricatedContext(t *testing.T) {
	t.Parallel()
	history := &domain.ConversationHistory{Messages: []domain.Message{
		{Role: contextUserRole, Kind: "initial_prompt", Content: "first request"},
		{Role: contextAssistantRole, Kind: "response", Content: `{"commands":["echo first"]}`},
		{Role: contextUserRole, Kind: "command_context", Content: "[COMMAND]\necho first\n[/COMMAND]\n\n[COMMAND_ERROR]\nexit status 1\n[/COMMAND_ERROR]"},
		{Role: contextUserRole, Kind: "refinement", Content: "try again"},
		{Role: contextUserRole, Kind: "project_context", Content: "project files"},
		{Role: contextUserRole, Kind: "command_error", Content: "provider error"},
	}}

	got := providerHistory(history)
	joined := ""
	var joinedSb47 strings.Builder
	for _, message := range got.Messages {
		joinedSb47.WriteString(message.Content)
	}
	joined += joinedSb47.String()
	for _, want := range []string{contextConversationSection, "first request", "try again", contextCommandSection, "echo first", "[PROJECT_CONTEXT]", "project files", contextErrorSection, contextExitError, "provider error"} {
		if !strings.Contains(joined, want) {
			t.Errorf("formatted typed history %q does not contain %q", joined, want)
		}
	}
	if strings.Contains(joined, "No prior conversation context was supplied") || strings.Contains(joined, "No command error context was supplied") {
		t.Fatalf("formatted typed history fabricated missing context: %q", joined)
	}
	if strings.Contains(joined, "[COMMAND]\n[COMMAND]") || strings.Contains(joined, "[COMMAND_ERROR]\n[COMMAND_ERROR]") {
		t.Fatalf("formatted typed history nested tags: %q", joined)
	}
}

func TestProviderHistoryAddsSectionsOnlyForProjectContext(t *testing.T) {
	t.Parallel()
	const contextRequest = "request"
	plain := &domain.ConversationHistory{SystemPrompt: "environment", Messages: []domain.Message{{Role: contextUserRole, Content: contextRequest}}}
	if got := providerHistory(plain); got != plain {
		t.Fatal("history without project context was copied or formatted")
	}
	withProject := &domain.ConversationHistory{SystemPrompt: "environment [PROJECT_CONTEXT] file [/PROJECT_CONTEXT]", Messages: []domain.Message{{Role: contextUserRole, Content: contextRequest}, {Role: contextAssistantRole, Content: "command"}}}
	got := providerHistory(withProject)
	for _, want := range []string{"[USER_REQUEST]", contextCommandSection, contextConversationSection, contextErrorSection, "request", "command"} {
		if !strings.Contains(got.Messages[0].Content+got.Messages[1].Content, want) {
			t.Fatalf("formatted history %q does not contain %q", got.Messages, want)
		}
	}
	if withProject.Messages[0].Content != "request" {
		t.Fatal("provider formatting mutated caller history")
	}
}
