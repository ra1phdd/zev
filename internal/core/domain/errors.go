package domain

import "errors"

var (
	// ErrProviderNotConfigured indicates that no inference provider is configured.
	ErrProviderNotConfigured = errors.New("provider not configured")
	// ErrInvalidPrompt indicates that the prompt is empty.
	ErrInvalidPrompt = errors.New("prompt cannot be empty")
	// ErrNoResponse indicates that the LLM returned no response.
	ErrNoResponse = errors.New("no response from LLM")
)
