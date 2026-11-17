package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/domain"
)

// Lifecycle records one query conversation through the consumer's store.
// A lifecycle is used serially for a single conversation.
type Lifecycle struct {
	store           Store
	conversationID  string
	provider, model string
	commandIDs      []string
}

// NewLifecycle creates a recorder using the supplied persistence port.
func NewLifecycle(store Store) *Lifecycle { return &Lifecycle{store: store} }

// StartHistoryInput supplies the current prompt and optional inherited user and
// assistant transcript. Provider and Model describe the current provider call.
type StartHistoryInput struct {
	Prompt, Provider, Model string
	ConversationHistory     []domain.Message
}

type continuationMetadata struct {
	ConversationHistory []domain.Message `json:"conversation_history,omitempty"` //nolint:tagliatelle // Preserve the optional persisted history format.
}

// Start records the initial user message before the first provider call.
func (l *Lifecycle) Start(ctx context.Context, prompt, provider, model string) error {
	return l.StartWithHistory(ctx, StartHistoryInput{Prompt: prompt, Provider: provider, Model: model})
}

// StartWithHistory atomically records the exact current prompt and a snapshot of
// the optional inherited transcript. Storage errors are wrapped with context.
func (l *Lifecycle) StartWithHistory(ctx context.Context, input StartHistoryInput) error {
	var metadata string
	if len(input.ConversationHistory) > 0 {
		if err := ValidateConversationHistory(input.ConversationHistory); err != nil {
			return err
		}
		payload, err := json.Marshal(continuationMetadata{ConversationHistory: input.ConversationHistory})
		if err != nil {
			return fmt.Errorf("%w: encode conversation history", ErrInvalid)
		}
		metadata = string(payload)
	}
	id := uuid.NewString()
	_, err := l.store.StartConversation(ctx, StartConversationInput{
		Conversation:   Conversation{ID: id, Kind: "query", Status: "started", Provider: input.Provider, Model: input.Model, MetadataJSON: metadata},
		InitialMessage: Message{ID: uuid.NewString(), ConversationID: id, Role: typeUser, Kind: initialPromptKind, Content: input.Prompt},
	})
	if err != nil {
		return fmt.Errorf("start history conversation: %w", err)
	}
	l.conversationID, l.provider, l.model = id, input.Provider, input.Model
	return nil
}

// Refine records the exact user refinement before the next provider call.
func (l *Lifecycle) Refine(ctx context.Context, content string) error {
	if l.conversationID == "" {
		return fmt.Errorf("%w: conversation has not started", ErrInvalid)
	}
	if content == "" {
		return fmt.Errorf("%w: refinement is empty", ErrInvalid)
	}
	_, err := l.store.AppendMessage(ctx, AppendMessageInput{Message: Message{ID: uuid.NewString(), ConversationID: l.conversationID, Role: typeUser, Kind: refinementKind, Content: content}})
	if err != nil {
		return fmt.Errorf("record refinement: %w", err)
	}
	return nil
}

// Response atomically records the structured response and all alternatives.
func (l *Lifecycle) Response(ctx context.Context, response *domain.OptionsResponse) error {
	if err := response.Validate(); err != nil {
		return fmt.Errorf("validate history response: %w", err)
	}
	l.commandIDs = nil
	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode history response: %w", err)
	}
	messageID, responseID := uuid.NewString(), uuid.NewString()
	commands := make([]Command, len(response.Commands))
	ids := make([]string, len(commands))
	for i, cmd := range response.Commands {
		ids[i] = uuid.NewString()
		commands[i] = Command{ID: ids[i], ResponseID: responseID, ConversationID: l.conversationID, Ordinal: i, Command: cmd.Command, ShortExplanation: cmd.ShortExplanation, IsDangerous: cmd.IsDangerous, DangerousExplanation: cmd.DangerousExplanation}
	}
	_, err = l.store.RecordResponse(ctx, RecordResponseInput{
		Message:  Message{ID: messageID, ConversationID: l.conversationID, Role: assistantRole, Kind: responseKind, Content: string(payload)},
		Response: Response{ID: responseID, ConversationID: l.conversationID, MessageID: messageID, Provider: l.provider, Model: l.model, PayloadJSON: string(payload), IsValid: response.IsValid, Commands: commands},
	})
	if err != nil {
		return fmt.Errorf("record history response: %w", err)
	}
	l.commandIDs = ids
	return nil
}

// SelectionOutcome distinguishes observed execution from terminal delivery.
type SelectionOutcome struct {
	Index             int
	DeliveryOutcome   string
	ExecutedAt        *time.Time
	ExecutionExitCode *int
	ExecutionError    string
}

// Select records a command choice and its independently observed delivery outcome.
func (l *Lifecycle) Select(ctx context.Context, outcome SelectionOutcome) error {
	if outcome.Index < 0 || outcome.Index >= len(l.commandIDs) {
		return fmt.Errorf("%w: command index", ErrInvalid)
	}
	err := l.store.RecordSelection(ctx, RecordSelectionInput{ConversationID: l.conversationID, CommandID: l.commandIDs[outcome.Index], SelectedAt: time.Now(), ExecutedAt: outcome.ExecutedAt, ExecutionExitCode: outcome.ExecutionExitCode, ExecutionError: outcome.ExecutionError, DeliveryOutcome: outcome.DeliveryOutcome})
	if err != nil {
		return fmt.Errorf("record selection: %w", err)
	}
	return nil
}

// Append records refinement, project_context, or command_error content after
// Start. It returns ErrInvalid for unsupported kinds, blank or invalid UTF-8
// content, or an unstarted lifecycle; storage failures are wrapped with context.
// Contextual kinds use refinement rows with optional metadata so files/log-only
// requests remain eligible for continuation without changing the database schema.
func (l *Lifecycle) Append(ctx context.Context, content, kind string) error {
	if l.conversationID == "" {
		return fmt.Errorf("%w: conversation has not started", ErrInvalid)
	}
	if !utf8.ValidString(content) || strings.TrimSpace(content) == "" {
		return fmt.Errorf("%w: invalid history message", ErrInvalid)
	}
	message := Message{ID: uuid.NewString(), ConversationID: l.conversationID, Role: typeUser, Kind: refinementKind, Content: content}
	switch kind {
	case refinementKind:
	case projectContextKind, commandErrorKind:
		payload, err := json.Marshal(struct {
			Kind string `json:"kind"`
		}{Kind: kind})
		if err != nil {
			return fmt.Errorf("%w: encode history message metadata", ErrInvalid)
		}
		message.MetadataJSON = string(payload)
	default:
		return fmt.Errorf("%w: unsupported history message kind", ErrInvalid)
	}
	if _, err := l.store.AppendMessage(ctx, AppendMessageInput{Message: message}); err != nil {
		return fmt.Errorf("append history message: %w", err)
	}
	return nil
}

// Complete marks the conversation completed, cancelled, or failed.
func (l *Lifecycle) Complete(ctx context.Context, status string) error {
	if l.conversationID == "" {
		return nil
	}
	if err := l.store.CompleteConversation(ctx, CompleteConversationInput{ConversationID: l.conversationID, Status: status, CompletedAt: time.Now()}); err != nil {
		return fmt.Errorf("complete history: %w", err)
	}
	return nil
}
