package ollama

import (
	"errors"

	"github.com/ra1phdd/zev-go/internal/entities/query/repository/openai"
)

// New создаёт новый Ollama провайдер (использует OpenAI-compatible API).
func New(baseURL, model string) (*openai.Provider, error) {
	if baseURL == "" {
		return nil, errors.New("OLLAMA_BASE_URL is required. Run `zev setup` to configure")
	}
	if model == "" {
		return nil, errors.New("OLLAMA_MODEL is required in config")
	}

	// Ollama использует OpenAI-compatible API, но не требует API key
	return openai.New("ollama", model, baseURL)
}
