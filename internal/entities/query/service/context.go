package service

import (
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func providerHistory(history *domain.ConversationHistory) *domain.ConversationHistory {
	if history == nil {
		return nil
	}
	if !strings.Contains(history.SystemPrompt, "[PROJECT_CONTEXT]") {
		return history
	}
	prepared := &domain.ConversationHistory{SystemPrompt: history.SystemPrompt, Messages: make([]domain.Message, len(history.Messages))}
	copy(prepared.Messages, history.Messages)
	for i := range prepared.Messages {
		switch prepared.Messages[i].Role {
		case "user":
			prepared.Messages[i].Content = "[USER_REQUEST]\n" + prepared.Messages[i].Content + "\n[/USER_REQUEST]\n\n[CONVERSATION]\nNo prior command or conversation context was supplied.\n[/CONVERSATION]\n\n[COMMAND_ERROR]\nNo command error context was supplied.\n[/COMMAND_ERROR]"
		case "assistant":
			prepared.Messages[i].Content = "[COMMAND]\n" + prepared.Messages[i].Content + "\n[/COMMAND]"
		}
	}
	return prepared
}
