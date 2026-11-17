package service

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	initialPromptKind  = "initial_prompt"
	refinementKind     = "refinement"
	responseKind       = "response"
	commandContextKind = "command_context"
	projectContextKind = "project_context"
	commandErrorKind   = "command_error"
	assistantRole      = "assistant"
)

// RestoreConversation reconstructs user turns and compact assistant responses in
// sequence order, excluding stored system context. Invalid response data returns
// an error classified with ErrInvalid without including conversation content.
func RestoreConversation(view ConversationView) ([]domain.Message, error) {
	messages := slices.Clone(view.Messages)
	slices.SortFunc(messages, func(a, b Message) int {
		return cmp.Compare(a.Sequence, b.Sequence)
	})
	responses := indexResponses(view.Responses)
	result, err := inheritedConversation(view.Conversation.MetadataJSON)
	if err != nil {
		return nil, err
	}
	for _, message := range messages {
		restored, err := restoreMessage(message, responses, view.Conversation.SelectedCommandID)
		if err != nil {
			return nil, err
		}
		result = append(result, restored...)
	}
	if err := ValidateConversationHistory(result); err != nil {
		return nil, err
	}
	return result, nil
}

func indexResponses(items []Response) map[string]Response {
	responses := make(map[string]Response, len(items))
	for _, response := range items {
		responses[response.MessageID] = response
	}
	return responses
}

func restoreMessage(message Message, responses map[string]Response, selectedCommandID string) ([]domain.Message, error) {
	switch {
	case message.Role == typeUser && (message.Kind == initialPromptKind || message.Kind == refinementKind):
		return restoreUserMessage(message)
	case message.Role == assistantRole && message.Kind == responseKind:
		return restoreResponseMessage(message, responses, selectedCommandID)
	default:
		return nil, nil
	}
}

func restoreUserMessage(message Message) ([]domain.Message, error) {
	if strings.TrimSpace(message.Content) == "" {
		return nil, nil
	}
	kind, err := restoredContextKind(message.MetadataJSON)
	if err != nil {
		return nil, err
	}
	return []domain.Message{{Role: typeUser, Content: message.Content, Kind: kind}}, nil
}

func restoredContextKind(metadata string) (string, error) {
	if metadata == "" {
		return "", nil
	}
	var fields struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal([]byte(metadata), &fields) != nil {
		return "", fmt.Errorf("%w: invalid message metadata", ErrInvalid)
	}
	switch fields.Kind {
	case projectContextKind, commandErrorKind:
		return fields.Kind, nil
	default:
		return "", nil
	}
}

func restoreResponseMessage(message Message, responses map[string]Response, selectedCommandID string) ([]domain.Message, error) {
	response, ok := responses[message.ID]
	if !ok || !utf8.ValidString(response.PayloadJSON) {
		return nil, fmt.Errorf("%w: invalid conversation response", ErrInvalid)
	}
	var options domain.OptionsResponse
	if json.Unmarshal([]byte(response.PayloadJSON), &options) != nil || options.Validate() != nil {
		return nil, fmt.Errorf("%w: invalid conversation response", ErrInvalid)
	}
	result := []domain.Message{{Role: assistantRole, Kind: responseKind, Content: options.CompactJSON()}}
	for _, command := range response.Commands {
		if command.ID != "" && command.ID == selectedCommandID {
			result = append(result, domain.Message{Role: typeUser, Kind: commandContextKind, Content: selectionContext(command)})
		}
	}
	return result, nil
}

func selectionContext(command Command) string {
	var sections []string
	sections = append(sections, "[COMMAND]\n"+command.Command+"\n[/COMMAND]")
	sections = append(sections, "Observed delivery outcome: "+command.DeliveryOutcome)
	if command.ExecutionExitCode != nil {
		sections = append(sections, fmt.Sprintf("Observed exit code: %d", *command.ExecutionExitCode))
	}
	if command.ExecutionError != "" {
		sections = append(sections, "[COMMAND_ERROR]\n"+command.ExecutionError+"\n[/COMMAND_ERROR]")
	}
	return strings.Join(sections, "\n\n")
}

func inheritedConversation(metadata string) ([]domain.Message, error) {
	if metadata == "" {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if !utf8.ValidString(metadata) || json.Unmarshal([]byte(metadata), &fields) != nil {
		return nil, fmt.Errorf("%w: invalid conversation metadata", ErrInvalid)
	}
	raw, exists := fields["conversation_history"]
	if !exists {
		return nil, nil
	}
	var messages []domain.Message
	if strings.TrimSpace(string(raw)) == "null" || json.Unmarshal(raw, &messages) != nil {
		return nil, fmt.Errorf("%w: invalid inherited transcript", ErrInvalid)
	}
	if len(messages) > 0 {
		if err := ValidateConversationHistory(messages); err != nil {
			return nil, err
		}
	}
	return messages, nil
}

// ValidateConversationHistory checks the complete transcript before it is sent to a provider.
// It rejects unsupported roles or kinds, blank content, and transcripts exceeding
// the fixed message and byte bounds. Errors are classified with ErrInvalid and do
// not include conversation content.
func ValidateConversationHistory(messages []domain.Message) error {
	if len(messages) > 512 {
		return fmt.Errorf("%w: conversation has too many messages", ErrInvalid)
	}
	var total int
	var hasUser bool
	for _, message := range messages {
		if (message.Role != typeUser && message.Role != assistantRole) || !validConversationKind(message.Kind) || !utf8.ValidString(message.Content) || strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("%w: invalid conversation message", ErrInvalid)
		}
		total += len(message.Content)
		if total > 1<<20 {
			return fmt.Errorf("%w: conversation is too large", ErrInvalid)
		}
		hasUser = hasUser || message.Role == typeUser
	}
	if !hasUser {
		return fmt.Errorf("%w: conversation requires a user message", ErrInvalid)
	}
	return nil
}

func validConversationKind(kind string) bool {
	switch kind {
	case "", initialPromptKind, refinementKind, responseKind, commandContextKind, projectContextKind, commandErrorKind:
		return true
	default:
		return false
	}
}
