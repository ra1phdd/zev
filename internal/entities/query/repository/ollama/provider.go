package ollama

import (
	"errors"
	"net/http"

	"github.com/ra1phdd/zev/internal/entities/query/repository/openai"
)

// New создаёт новый Ollama провайдер (использует OpenAI-compatible API).
func New(baseURL, model string, httpClient *http.Client) (*openai.Provider, error) {
	if baseURL == "" {
		return nil, errors.New("OLLAMA_BASE_URL is required. Run `zev setup` to configure")
	}
	if model == "" {
		return nil, errors.New("OLLAMA_MODEL is required in config")
	}
	if httpClient == nil {
		return nil, errors.New("httpClient is required")
	}

	// Ollama использует OpenAI-compatible API, но не требует API key
	return openai.New("ollama", model, baseURL, httpClient)
}
