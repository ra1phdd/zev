package domain

import "errors"

var (
	ErrProviderNotConfigured = errors.New("provider not configured")
	ErrInvalidPrompt         = errors.New("prompt cannot be empty")
	ErrNoResponse            = errors.New("no response from LLM")
)
