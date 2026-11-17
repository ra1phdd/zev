package service

import (
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	contextInitialPromptKind  = "initial_prompt"
	contextRefinementKind     = "refinement"
	contextResponseKind       = "response"
	contextCommandContextKind = "command_context"
	contextProjectContextKind = "project_context"
	contextCommandErrorKind   = "command_error"
	contextUserRole           = "user"
	contextAssistantRole      = "assistant"
)

func providerHistory(history *domain.ConversationHistory) *domain.ConversationHistory {
	if history == nil {
		return nil
	}
	formatKinds := false
	for _, message := range history.Messages {
		if message.Kind != "" {
			formatKinds = true
			break
		}
	}
	if !formatKinds && !strings.Contains(history.SystemPrompt, "[PROJECT_CONTEXT]") {
		return history
	}
	prepared := &domain.ConversationHistory{SystemPrompt: history.SystemPrompt, Messages: make([]domain.Message, len(history.Messages))}
	copy(prepared.Messages, history.Messages)
	for i := range prepared.Messages {
		message := &prepared.Messages[i]
		switch message.Kind {
		case contextInitialPromptKind, contextRefinementKind:
			message.Content = section("CONVERSATION", message.Content)
		case contextProjectContextKind:
			message.Content = section("PROJECT_CONTEXT", message.Content)
		case contextCommandErrorKind:
			message.Content = section("COMMAND_ERROR", message.Content)
		case contextResponseKind:
			message.Content = section("COMMAND", message.Content)
		case contextCommandContextKind:
			message.Content = commandContextSection(message.Content)
		default:
			switch message.Role {
			case contextUserRole:
				message.Content = userRequestSection(message.Content)
			case contextAssistantRole:
				message.Content = section("COMMAND", message.Content)
			}
		}
	}
	return prepared
}

func section(name, content string) string {
	return "[" + name + "]\n" + content + "\n[/" + name + "]"
}

func commandContextSection(content string) string {
	if strings.Contains(content, "[COMMAND]") || strings.Contains(content, "[COMMAND_ERROR]") {
		return content
	}
	return section("COMMAND", content)
}

func userRequestSection(content string) string {
	return section("USER_REQUEST", content) + "\n\n" + section("CONVERSATION", "No prior command or conversation context was supplied.") + "\n\n" + section("COMMAND_ERROR", "No command error context was supplied.")
}
