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
	client *anthropic.Client
	model  string
}

// New создаёт новый Anthropic провайдер.
func New(apiKey, model string, httpClient *http.Client) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required. Run `zev setup` to configure")
	}
	if model == "" {
		return nil, errors.New("ANTHROPIC_MODEL is required in config")
	}
	if httpClient == nil {
		return nil, errors.New("httpClient is required")
	}

	client := anthropic.NewClient(
		apiKey,
		anthropic.WithHTTPClient(httpClient),
	)

	return &Provider{
		client: client,
		model:  model,
	}, nil
}

// GetOptions получает варианты команд от Anthropic.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	assembledPrompt := fmt.Sprintf(repository.PromptTemplate, envContext, prompt)

	resp, err := p.client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model: anthropic.Model(p.model),
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage(assembledPrompt),
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

// Name возвращает имя провайдера.
func (p *Provider) Name() string {
	return "anthropic"
}

// Model возвращает используемую модель.
func (p *Provider) Model() string {
	return p.model
}
