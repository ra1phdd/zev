package service

import (
	"maps"

	"github.com/ra1phdd/zev/internal/core/config"
)

// mergeProviderChanges applies only edited provider fields, leaving concurrent
// changes to unrelated or unchanged settings intact.
// The provider identifier is validated by the setup selection list before this helper is called.
func mergeProviderChanges(latest, original, changed *config.Config, provider string) {
	latest.LLMProvider = provider
	merge := func(target *string, before, after string) {
		if before != after {
			*target = after
		}
	}
	switch provider {
	case providerOpenAI, providerOpenCode:
		merge(&latest.OpenAIAPIKey, original.OpenAIAPIKey, changed.OpenAIAPIKey)
		merge(&latest.OpenAIModel, original.OpenAIModel, changed.OpenAIModel)
		merge(&latest.OpenAIBaseURL, original.OpenAIBaseURL, changed.OpenAIBaseURL)
		if provider == providerOpenCode && !maps.Equal(original.HTTPHeaders, changed.HTTPHeaders) {
			latest.HTTPHeaders = maps.Clone(changed.HTTPHeaders)
		}
	case providerGemini:
		merge(&latest.GeminiAPIKey, original.GeminiAPIKey, changed.GeminiAPIKey)
		merge(&latest.GeminiModel, original.GeminiModel, changed.GeminiModel)
		merge(&latest.GeminiBaseURL, original.GeminiBaseURL, changed.GeminiBaseURL)
	case providerAnthropic:
		merge(&latest.AnthropicAPIKey, original.AnthropicAPIKey, changed.AnthropicAPIKey)
		merge(&latest.AnthropicModel, original.AnthropicModel, changed.AnthropicModel)
		merge(&latest.AnthropicBaseURL, original.AnthropicBaseURL, changed.AnthropicBaseURL)
	case providerOllama:
		merge(&latest.OllamaModel, original.OllamaModel, changed.OllamaModel)
		merge(&latest.OllamaBaseURL, original.OllamaBaseURL, changed.OllamaBaseURL)
	}
}
