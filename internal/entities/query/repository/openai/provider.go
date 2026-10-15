package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
	"github.com/sashabaranov/go-openai"
)

// Provider реализует domain.InferenceProvider для OpenAI API.
type Provider struct {
	client  *openai.Client
	model   string
	baseURL string
}

// New создаёт новый OpenAI провайдер.
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

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	config.HTTPClient = httpClient
	client := openai.NewClientWithConfig(config)

	return &Provider{
		client:  client,
		model:   model,
		baseURL: baseURL,
	}, nil
}

// GetOptions получает варианты команд от OpenAI.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	assembledPrompt := fmt.Sprintf(repository.PromptTemplate, envContext, prompt)

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: assembledPrompt},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, domain.ErrNoResponse
	}

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// Name возвращает имя провайдера.
func (p *Provider) Name() string {
	if p.baseURL != "https://api.openai.com/v1" {
		return "openai-compatible"
	}
	return "openai"
}

// Model возвращает используемую модель.
func (p *Provider) Model() string {
	return p.model
}
