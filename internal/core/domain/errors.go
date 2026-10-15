package domain

import "errors"

var (
	// ErrProviderNotConfigured возвращается, когда провайдер не настроен.
	ErrProviderNotConfigured = errors.New("provider not configured")

	// ErrInvalidPrompt возвращается, когда промпт пустой.
	ErrInvalidPrompt = errors.New("prompt cannot be empty")

	// ErrNoResponse возвращается, когда LLM не вернул ответ.
	ErrNoResponse = errors.New("no response from LLM")
)
