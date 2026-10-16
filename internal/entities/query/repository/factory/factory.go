package factory

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository/anthropic"
	"github.com/ra1phdd/zev/internal/entities/query/repository/gemini"
	"github.com/ra1phdd/zev/internal/entities/query/repository/ollama"
	"github.com/ra1phdd/zev/internal/entities/query/repository/openai"
	"github.com/ra1phdd/zev/internal/entities/query/repository/opencode"
)

// NewProvider создаёт провайдер на основе конфигурации.
func NewProvider(cfg *config.Config, httpClient *http.Client) (domain.InferenceProvider, error) {
	if httpClient == nil {
		return nil, errors.New("httpClient is required")
	}

	switch cfg.LLMProvider {
	case "openai":
		return openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL, httpClient)
	case "gemini":
		return gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, httpClient)
	case "anthropic":
		return anthropic.New(cfg.AnthropicAPIKey, cfg.AnthropicModel, cfg.AnthropicBaseURL, httpClient)
	case "ollama":
		return ollama.New(cfg.OllamaBaseURL, cfg.OllamaModel, httpClient)
	case "opencode":
		return opencode.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL, httpClient)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.LLMProvider)
	}
}
