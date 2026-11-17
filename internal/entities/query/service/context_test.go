package service

import (
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestProviderHistoryAddsSectionsOnlyForProjectContext(t *testing.T) {
	t.Parallel()
	const contextRequest = "request"
	plain := &domain.ConversationHistory{SystemPrompt: "environment", Messages: []domain.Message{{Role: "user", Content: contextRequest}}}
	if got := providerHistory(plain); got != plain {
		t.Fatal("history without project context was copied or formatted")
	}
	withProject := &domain.ConversationHistory{SystemPrompt: "environment [PROJECT_CONTEXT] file [/PROJECT_CONTEXT]", Messages: []domain.Message{{Role: "user", Content: contextRequest}, {Role: "assistant", Content: "command"}}}
	got := providerHistory(withProject)
	for _, want := range []string{"[USER_REQUEST]", "[COMMAND]", "[CONVERSATION]", "[COMMAND_ERROR]", "request", "command"} {
		if !strings.Contains(got.Messages[0].Content+got.Messages[1].Content, want) {
			t.Fatalf("formatted history %q does not contain %q", got.Messages, want)
		}
	}
	if withProject.Messages[0].Content != "request" {
		t.Fatal("provider formatting mutated caller history")
	}
}
