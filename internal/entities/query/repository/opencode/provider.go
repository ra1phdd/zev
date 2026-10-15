package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
)

// Provider реализует domain.InferenceProvider для OpenCode API.
type Provider struct {
	client  *http.Client
	apiKey  string
	model   string
	baseURL string
}

// New создаёт новый OpenCode провайдер.
func New(apiKey, model, baseURL string, httpClient *http.Client) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is required. Run `zev setup` to configure")
	}
	if model == "" {
		return nil, errors.New("OPENAI_MODEL is required in config")
	}
	if baseURL == "" {
		return nil, errors.New("OPENAI_BASE_URL is required in config")
	}
	if httpClient == nil {
		return nil, errors.New("httpClient is required")
	}

	return &Provider{
		client:  httpClient,
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
	}, nil
}

// openCodeRequest представляет запрос к OpenCode API.
type openCodeRequest struct {
	Model               string                 `json:"model"`
	Messages            []openCodeMessage      `json:"messages"`
	Tools               []any                  `json:"tools"`
	Stream              bool                   `json:"stream"`
	StreamOptions       openCodeStreamOptions  `json:"stream_options"`
	MaxCompletionTokens int                    `json:"max_completion_tokens"`
	ResponseFormat      openCodeResponseFormat `json:"response_format"`
}

type openCodeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openCodeStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openCodeResponseFormat struct {
	Type string `json:"type"`
}

// openCodeResponse представляет ответ от OpenCode API.
type openCodeResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// GetOptions получает варианты команд от OpenCode.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	assembledPrompt := fmt.Sprintf(repository.PromptTemplate, envContext, prompt)

	reqBody := openCodeRequest{
		Model: p.model,
		Messages: []openCodeMessage{
			{Role: "user", Content: assembledPrompt},
		},
		Tools:               []any{},
		Stream:              false, // Для синхронного режима
		StreamOptions:       openCodeStreamOptions{IncludeUsage: true},
		MaxCompletionTokens: 32000,
		ResponseFormat:      openCodeResponseFormat{Type: "json_object"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenCode API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenCode API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp openCodeResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenCode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, domain.ErrNoResponse
	}

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(apiResp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse command options: %w", err)
	}

	return &result, nil
}

// Name возвращает имя провайдера.
func (p *Provider) Name() string {
	return "opencode"
}

// Model возвращает используемую модель.
func (p *Provider) Model() string {
	return p.model
}
