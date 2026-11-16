// Package factory selects a query provider from application configuration.
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
		provider, err := openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL, httpClient)
		if err != nil {
			return nil, fmt.Errorf("creating openai provider: %w", err)
		}
		return provider, nil
	case "gemini":
		provider, err := gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, httpClient)
		if err != nil {
			return nil, fmt.Errorf("creating gemini provider: %w", err)
		}
		return provider, nil
	case "anthropic":
		provider, err := anthropic.New(cfg.AnthropicAPIKey, cfg.AnthropicModel, cfg.AnthropicBaseURL, httpClient)
		if err != nil {
			return nil, fmt.Errorf("creating anthropic provider: %w", err)
		}
		return provider, nil
	case "ollama":
		provider, err := ollama.New(cfg.OllamaBaseURL, cfg.OllamaModel, httpClient)
		if err != nil {
			return nil, fmt.Errorf("creating ollama provider: %w", err)
		}
		return provider, nil
	case "opencode":
		provider, err := opencode.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL, httpClient)
		if err != nil {
			return nil, fmt.Errorf("creating opencode provider: %w", err)
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.LLMProvider)
	}
}
