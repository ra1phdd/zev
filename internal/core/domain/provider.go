package domain

import "context"

// InferenceProvider определяет интерфейс для всех LLM провайдеров.
type InferenceProvider interface {
	// GetOptions получает варианты команд от LLM (базовый метод)
	GetOptions(ctx context.Context, prompt string, envContext string) (*OptionsResponse, error)

	// GetOptionsWithHistory получает варианты команд с учётом истории диалога (для уточнения)
	GetOptionsWithHistory(ctx context.Context, history *ConversationHistory) (*OptionsResponse, error)

	// Name возвращает имя провайдера (openai, gemini, ollama, openai-compatible)
	Name() string

	// Model возвращает используемую модель
	Model() string
}
