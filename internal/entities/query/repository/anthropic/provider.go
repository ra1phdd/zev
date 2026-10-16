package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/liushuangls/go-anthropic/v2"
	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
)

type Provider struct {
	client  *anthropic.Client
	model   string
	baseURL string
}

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

func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	systemPrompt := fmt.Sprintf(repository.SystemPrompt, envContext)
	userMessage := fmt.Sprintf(repository.UserPromptTemplate, prompt)

	resp, err := p.client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:  anthropic.Model(p.model),
		System: systemPrompt,
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

	// Извлекаем JSON из markdown блока, если он там есть
	jsonContent := extractJSON(textContent)

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

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
		return nil, repository.WrapNetworkError(p.Name(), err)
	}

	textContent := resp.GetFirstContentText()
	if textContent == "" {
		return nil, domain.ErrNoResponse
	}

	// Извлекаем JSON из markdown блока, если он там есть
	jsonContent := extractJSON(textContent)

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (p *Provider) Name() string {
	return "anthropic"
}

func (p *Provider) Model() string {
	return p.model
}

// extractJSON извлекает JSON из markdown блока или возвращает текст как есть.
func extractJSON(text string) string {
	// Попробуем найти JSON блок в markdown (```json ... ```)
	const jsonPrefix = "```json"
	const jsonSuffix = "```"

	startIdx := strings.Index(text, jsonPrefix)
	if startIdx == -1 {
		// Нет markdown блока, возвращаем текст как есть
		return text
	}

	startIdx += len(jsonPrefix)
	endIdx := strings.Index(text[startIdx:], jsonSuffix)
	if endIdx == -1 {
		// Не нашли закрывающий блок, возвращаем текст как есть
		return text
	}

	return strings.TrimSpace(text[startIdx : startIdx+endIdx])
}
