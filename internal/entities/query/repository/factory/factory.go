package factory

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/gemini"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/ollama"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository/openai"
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
		return gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel, httpClient)
	case "ollama":
		return ollama.New(cfg.OllamaBaseURL, cfg.OllamaModel, httpClient)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.LLMProvider)
	}
}
