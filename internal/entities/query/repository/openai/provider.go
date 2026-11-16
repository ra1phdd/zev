// Package openai adapts OpenAI-compatible APIs to the query provider port.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	"github.com/sashabaranov/go-openai"
)

// Provider calls OpenAI-compatible and decodes query options.
type Provider struct {
	client  *openai.Client
	model   string
	baseURL string
}

// New creates a OpenAI-compatible provider.
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

// GetOptions requests query options for a new conversation.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	systemPrompt := fmt.Sprintf(repository.SystemPrompt, envContext)
	userMessage := prompt

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userMessage},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", p.Name(), repository.WrapNetworkError(p.Name(), err))
	}

	if len(resp.Choices) == 0 {
		return nil, domain.ErrNoResponse
	}

	rawContent := resp.Choices[0].Message.Content
	cleanJSON := repository.ExtractJSON(rawContent)

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), repository.WrapResponseError(p.Name(), err))
	}

	return &result, nil
}

// GetOptionsWithHistory requests query options for an existing conversation.
func (p *Provider) GetOptionsWithHistory(ctx context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	messages := make([]openai.ChatCompletionMessage, 0, 1+len(history.Messages))
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: history.SystemPrompt,
	})

	for _, msg := range history.Messages {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: messages,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", p.Name(), repository.WrapNetworkError(p.Name(), err))
	}

	if len(resp.Choices) == 0 {
		return nil, domain.ErrNoResponse
	}

	rawContent := resp.Choices[0].Message.Content
	cleanJSON := repository.ExtractJSON(rawContent)

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), repository.WrapResponseError(p.Name(), err))
	}

	return &result, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	if p.baseURL != "https://api.openai.com/v1" {
		return "openai-compatible"
	}
	return "openai"
}

// Model returns the configured model identifier.
func (p *Provider) Model() string {
	return p.model
}
