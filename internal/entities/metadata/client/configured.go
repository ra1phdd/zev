package client

import (
	"errors"
	"net/http"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/entities/metadata/service"
)

// NewConfigured selects a dedicated metadata adapter from the configured provider.
// It neither loads configuration nor sends a request. The caller supplies the
// shared HTTP transport and must obtain consent before invoking generation.
func NewConfigured(cfg *config.Config, httpClient *http.Client) (service.MetadataGenerator, error) {
	if cfg == nil {
		return nil, errors.New("invalid metadata provider configuration")
	}
	selected := Config{Provider: ProviderOpenAICompatible}
	switch cfg.LLMProvider {
	case "openai", "opencode":
		selected.BaseURL, selected.Model, selected.APIKey = cfg.OpenAIBaseURL, cfg.OpenAIModel, cfg.OpenAIAPIKey
	case "ollama":
		selected.BaseURL, selected.Model = cfg.OllamaBaseURL, cfg.OllamaModel
	case providerGemini:
		selected.Provider = providerGemini
		selected.BaseURL, selected.Model, selected.APIKey = cfg.GeminiBaseURL, cfg.GeminiModel, cfg.GeminiAPIKey
	case providerAnthropic:
		selected.Provider = providerAnthropic
		selected.BaseURL, selected.Model, selected.APIKey = cfg.AnthropicBaseURL, cfg.AnthropicModel, cfg.AnthropicAPIKey
	default:
		return nil, errors.New("unsupported metadata provider")
	}
	return New(selected, httpClient)
}
