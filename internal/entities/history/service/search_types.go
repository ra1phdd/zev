package service

import (
	"context"
	"time"
)

// SearchRequest selects a bounded, deterministic page of history documents.
// From and To define a half-open UTC interval. Literal and ExactCommand bypass expansion.
type SearchRequest struct {
	Query        string    `json:"query"`
	Mode         string    `json:"mode"`
	Fuzzy        string    `json:"fuzzy"`
	Limit        int       `json:"limit"`
	Cursor       string    `json:"cursor,omitempty"`
	From         time.Time `json:"from,omitzero"`
	To           time.Time `json:"to,omitzero"`
	Provider     string    `json:"provider,omitempty"`
	Model        string    `json:"model,omitempty"`
	Status       string    `json:"status,omitempty"`
	Type         string    `json:"type,omitempty"`
	Dangerous    *bool     `json:"dangerous,omitempty"`
	Literal      bool      `json:"literal"`
	ExactCommand string    `json:"exactCommand,omitempty"`
}

// SearchHit retains the actual matching document rather than substituting a command.
type SearchHit struct {
	DocumentID     string       `json:"documentId"`
	ConversationID string       `json:"conversationId"`
	MessageID      string       `json:"messageId,omitempty"`
	CommandID      string       `json:"commandId,omitempty"`
	DocumentType   string       `json:"documentType"`
	CreatedAt      time.Time    `json:"createdAt"`
	Snippet        string       `json:"snippet"`
	Score          float64      `json:"score"`
	ScoreKind      string       `json:"scoreKind"`
	MatchSources   []string     `json:"matchSources"`
	Conversation   Conversation `json:"conversation"`
}

// SearchResult explains ranking, fallback, candidate limits and pagination.
type SearchResult struct {
	Hits              []SearchHit       `json:"hits"`
	Mode              string            `json:"mode"`
	RequestedMode     string            `json:"requestedMode"`
	Degraded          bool              `json:"degraded"`
	Warning           string            `json:"warning,omitempty"`
	Truncated         bool              `json:"truncated"`
	NextCursor        string            `json:"nextCursor,omitempty"`
	ProfileID         string            `json:"profileId"`
	EmbeddingCoverage EmbeddingCoverage `json:"embeddingCoverage"`
}

// EmbeddingCoverage counts eligible user messages and current vectors.
type EmbeddingCoverage struct {
	Eligible int64 `json:"eligible"`
	Ready    int64 `json:"ready"`
	Failed   int64 `json:"failed"`
	Stale    int64 `json:"stale"`
}

// IndexStatus describes the currently selected local and vector profiles.
type IndexStatus struct {
	ProfileID          string            `json:"profileId"`
	Documents          int64             `json:"documents"`
	LexicalReady       bool              `json:"lexicalReady"`
	EmbeddingProfileID string            `json:"embeddingProfileId,omitempty"`
	EmbeddingCoverage  EmbeddingCoverage `json:"embeddingCoverage"`
}

// Embedder is the explicit, separately configured user-text embedding port.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	ProfileID() string
	Model() string
	Dimensions() int
}

// Embedding records one user-message vector in fixed little-endian float32 format.
type Embedding struct {
	MessageID   string
	Model       string
	Dimensions  int
	Vector      []byte
	ContentHash string
	CreatedAt   time.Time
	Status      string
	Error       string
	ProfileID   string
}

// VectorCandidate couples a current vector with its source user message.
type VectorCandidate struct {
	Hit        SearchHit
	Vector     []byte
	Dimensions int
}

// SearchStore supplies indexed lexical candidates and streaming vector access.
// VisitVectors must not retain the callback or load every vector into memory.
type SearchStore interface {
	LexicalSearch(ctx context.Context, request SearchRequest) (SearchResult, error)
	Revision(ctx context.Context) (int64, error)
	Coverage(ctx context.Context, profileID string) (EmbeddingCoverage, error)
	VisitVectors(ctx context.Context, request SearchRequest, profileID string, visit func(VectorCandidate) error) error
	PendingMessages(ctx context.Context, profileID string, retryFailed bool, afterID string, limit int) ([]Message, error)
	SaveEmbedding(ctx context.Context, embedding Embedding) error
}

// RebuildResult counts vector rebuild outcomes without exposing message contents.
type RebuildResult struct {
	Ready  int `json:"ready"`
	Failed int `json:"failed"`
}
