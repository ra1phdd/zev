package factory

import (
	"fmt"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/gemini"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/ollama"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/openai"
)

// NewProvider создаёт провайдер на основе конфигурации.
func NewProvider(cfg *config.Config) (domain.InferenceProvider, error) {
	switch cfg.LLMProvider {
	case "openai":
		return openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL)
	case "gemini":
		return gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel)
	case "ollama":
		return ollama.New(cfg.OllamaBaseURL, cfg.OllamaModel)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.LLMProvider)
	}
}
