package domain

import "context"

// InferenceProvider generates command suggestions using an LLM.
type InferenceProvider interface {
	GetOptions(ctx context.Context, prompt string, envContext string) (*OptionsResponse, error)
	GetOptionsWithHistory(ctx context.Context, history *ConversationHistory) (*OptionsResponse, error)
	Name() string
	Model() string
}
