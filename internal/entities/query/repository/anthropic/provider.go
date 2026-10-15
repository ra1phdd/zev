package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/liushuangls/go-anthropic/v2"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
)

// Provider реализует domain.InferenceProvider для Anthropic API.
type Provider struct {
	client  *anthropic.Client
	model   string
	baseURL string
}

// New создаёт новый Anthropic провайдер.
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
		anthropic.WithHTTPClient(httpClient),
		anthropic.WithBaseURL(baseURL),
	)

	return &Provider{
		client:  client,
		model:   model,
		baseURL: baseURL,
	}, nil
}

// GetOptions получает варианты команд от Anthropic.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	userMessage := fmt.Sprintf(repository.UserPromptTemplate, envContext, prompt)

	resp, err := p.client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:  anthropic.Model(p.model),
		System: repository.SystemPrompt,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage(userMessage),
		},
		MaxTokens: 2048,
	})

	if err != nil {
		return nil, repository.WrapNetworkError(p.Name(), err)
	}

	textContent := resp.GetFirstContentText()
	if textContent == "" {
		return nil, domain.ErrNoResponse
	}

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(textContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// GetOptionsWithHistory получает варианты команд с учётом истории диалога.
func (p *Provider) GetOptionsWithHistory(ctx context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	messages := []anthropic.Message{}

	// Добавляем историю сообщений
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
		return nil, repository.WrapNetworkError(p.Name(), err)
	}

	textContent := resp.GetFirstContentText()
	if textContent == "" {
		return nil, domain.ErrNoResponse
	}

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(textContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// Name возвращает имя провайдера.
func (p *Provider) Name() string {
	return "anthropic"
}

// Model возвращает используемую модель.
func (p *Provider) Model() string {
	return p.model
}
