// Package anthropic adapts the Anthropic messages API to the query provider port.
package anthropic

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/liushuangls/go-anthropic/v2"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
)

// Provider calls Anthropic and decodes query options.
type Provider struct {
	client  *anthropic.Client
	model   string
	baseURL string
}

// New creates an Anthropic provider.
func New(apiKey, model, baseURL string, httpClient *http.Client) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required. Run `zev setup` to configure")
	}
	if model == "" {
		return nil, errors.New("ANTHROPIC_MODEL is required in config")
	}
	if baseURL == "" {
		return nil, errors.New("ANTHROPIC_BASE_URL is required in config")
	}
	if httpClient == nil {
		return nil, errors.New("httpClient is required")
	}

	client := anthropic.NewClient(
		apiKey,
		anthropic.WithHTTPClient(repository.WithResponseBodyLimit(httpClient)),
		anthropic.WithBaseURL(baseURL),
	)

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

	resp, err := p.client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:  anthropic.Model(p.model),
		System: systemPrompt,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage(userMessage),
		},
		MaxTokens: 2048,
	})
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", p.Name(), repository.WrapNetworkError(p.Name(), err))
	}

	textContent := resp.GetFirstContentText()
	if textContent == "" {
		return nil, domain.ErrNoResponse
	}

	result, err := repository.DecodeOptionsResponse(p.Name(), textContent)
	if err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), err)
	}

	return result, nil
}

// GetOptionsWithHistory requests query options for an existing conversation.
func (p *Provider) GetOptionsWithHistory(ctx context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	messages := []anthropic.Message{}

	for _, msg := range history.Messages {
		switch msg.Role {
		case "user":
			messages = append(messages, anthropic.NewUserTextMessage(msg.Content))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantTextMessage(msg.Content))
		}
	}

	resp, err := p.client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     anthropic.Model(p.model),
		System:    history.SystemPrompt,
		Messages:  messages,
		MaxTokens: 2048,
	})
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", p.Name(), repository.WrapNetworkError(p.Name(), err))
	}

	textContent := resp.GetFirstContentText()
	if textContent == "" {
		return nil, domain.ErrNoResponse
	}

	result, err := repository.DecodeOptionsResponse(p.Name(), textContent)
	if err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), err)
	}

	return result, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "anthropic"
}

// Model returns the configured model identifier.
func (p *Provider) Model() string {
	return p.model
}
