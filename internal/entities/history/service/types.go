package service

import (
	"context"
	"errors"
	"time"
)

// Repository errors can be classified with errors.Is.
var (
	ErrNotFound          = errors.New("history: not found")
	ErrConflict          = errors.New("history: conflict")
	ErrInvalid           = errors.New("history: invalid input")
	ErrUnsupportedSchema = errors.New("history: unsupported schema")
	// ErrBusy marks a bounded wait that could not acquire SQLite storage.
	ErrBusy = errors.New("history: busy")
	// ErrCancelled marks a cancelled history operation.
	ErrCancelled = errors.New("history: cancelled")
	// ErrStorageFailure marks an I/O or database failure without exposing payloads.
	ErrStorageFailure = errors.New("history: storage failure")
	// ErrEmbeddingUnavailable marks a missing or failing optional embedding provider.
	ErrEmbeddingUnavailable = errors.New("history: embedding unavailable")
	// ErrInvalidQuery is the query-validation classification, also recognized as ErrInvalid.
	ErrInvalidQuery = ErrInvalid
)

// Conversation is the durable conversation header.
type Conversation struct {
	ID                                                             string
	CreatedAt, UpdatedAt                                           time.Time
	Status, Kind, Provider, Model, SelectedCommandID, MetadataJSON string
}

// Message is an immutable ordered conversation message.
type Message struct {
	ID, ConversationID      string
	Sequence                int64
	Role, Kind, Content     string
	CreatedAt               time.Time
	MetadataJSON, PartsJSON string
}

// Command is one alternative in a model response.
type Command struct {
	ID, ResponseID, ConversationID                  string
	Ordinal                                         int
	Command, ShortExplanation, DangerousExplanation string
	IsDangerous                                     bool
	SelectedAt, ExecutedAt                          *time.Time
	ExecutionExitCode                               *int
	ExecutionError                                  string
	DeliveryOutcome                                 string
}

// Response is the persisted structured provider response.
type Response struct {
	ID, ConversationID, MessageID, Provider, Model, PayloadJSON string
	IsValid                                                     bool
	CreatedAt                                                   time.Time
	Commands                                                    []Command
}

// ConversationView contains the complete ordered conversation.
type ConversationView struct {
	Conversation Conversation
	Messages     []Message
	Responses    []Response
}

// StartConversationInput supplies the conversation and its initial message.
type StartConversationInput struct {
	Conversation   Conversation
	InitialMessage Message
}

// AppendMessageInput supplies the immutable message to append.
type AppendMessageInput struct{ Message Message }

// RecordResponseInput supplies an assistant message and its response.
type RecordResponseInput struct {
	Message  Message
	Response Response
}

// RecordSelectionInput records a selected command and optional execution result.
type RecordSelectionInput struct {
	ConversationID, CommandID string
	SelectedAt                time.Time
	ExecutedAt                *time.Time
	ExecutionExitCode         *int
	ExecutionError            string
	DeliveryOutcome           string
}

// CompleteConversationInput sets the terminal conversation status.
type CompleteConversationInput struct {
	ConversationID, Status string
	CompletedAt            time.Time
}

// ListRequest specifies a bounded keyset page, ordered newest first.
type ListRequest struct {
	Limit           int
	BeforeCreatedAt time.Time
	BeforeID        string
}

// Store is the consumer-side history persistence port.
type Store interface {
	StartConversation(context.Context, StartConversationInput) (Conversation, error)
	AppendMessage(context.Context, AppendMessageInput) (Message, error)
	RecordResponse(context.Context, RecordResponseInput) (Response, error)
	RecordSelection(context.Context, RecordSelectionInput) error
	CompleteConversation(context.Context, CompleteConversationInput) error
	GetConversation(context.Context, string) (ConversationView, error)
	GetMessage(context.Context, string) (Message, error)
	ListConversations(context.Context, ListRequest) ([]Conversation, error)
	DeleteConversation(context.Context, string) error
}
