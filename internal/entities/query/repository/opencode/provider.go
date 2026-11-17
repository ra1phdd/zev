// Package opencode adapts the OpenCode streaming API to the query provider port.
package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	openaisdk "github.com/sashabaranov/go-openai"
)

// Provider calls OpenCode and decodes query options.
type Provider struct {
	client  *http.Client
	apiKey  string
	model   string
	baseURL string
}

// New creates an OpenCode provider.
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
		client:  repository.WithResponseBodyLimit(httpClient),
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
	}, nil
}

// openCodeRequest представляет запрос к OpenCode API.
type openCodeRequest struct {
	Model               string                  `json:"model"`
	Messages            []openCodeMessage       `json:"messages"`
	Stream              bool                    `json:"stream"`
	StreamOptions       openCodeStreamOptions   `json:"stream_options"`            //nolint:tagliatelle // OpenAI-compatible wire format uses snake_case.
	MaxCompletionTokens int                     `json:"max_completion_tokens"`     //nolint:tagliatelle // OpenAI-compatible wire format uses snake_case.
	ResponseFormat      *openCodeResponseFormat `json:"response_format,omitempty"` //nolint:tagliatelle // OpenAI-compatible wire format uses snake_case.
}

type openCodeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openCodeStreamOptions struct {
	IncludeUsage bool `json:"include_usage"` //nolint:tagliatelle // OpenAI-compatible wire format uses snake_case.
}

type openCodeResponseFormat struct {
	Type string `json:"type"`
}

// openCodeResponse представляет ответ от OpenCode API (streaming).
type openCodeStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"` //nolint:tagliatelle // OpenAI-compatible wire format uses snake_case.
	} `json:"choices"`
}

// GetOptions requests query options for a new conversation.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	systemPrompt := fmt.Sprintf(repository.SystemPrompt, envContext)
	userMessage := prompt

	reqBody := openCodeRequest{
		Model: p.model,
		Messages: []openCodeMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Stream:              true,
		StreamOptions:       openCodeStreamOptions{IncludeUsage: true},
		MaxCompletionTokens: 32000,
	}

	return p.sendRequest(ctx, reqBody)
}

// GetOptionsWithHistory requests query options for an existing conversation.
func (p *Provider) GetOptionsWithHistory(ctx context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	messages := make([]openCodeMessage, 0, 1+len(history.Messages))
	messages = append(messages, openCodeMessage{
		Role:    "system",
		Content: history.SystemPrompt,
	})

	for _, msg := range history.Messages {
		messages = append(messages, openCodeMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	reqBody := openCodeRequest{
		Model:               p.model,
		Messages:            messages,
		Stream:              true,
		StreamOptions:       openCodeStreamOptions{IncludeUsage: true},
		MaxCompletionTokens: 32000,
	}

	return p.sendRequest(ctx, reqBody)
}

// sendRequest отправляет запрос к OpenCode API и обрабатывает streaming-ответ.
func (p *Provider) sendRequest(ctx context.Context, reqBody openCodeRequest) (*domain.OptionsResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", repository.WrapNetworkError(p.Name(), err))
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Host", "opencode.ai")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenCode request failed: %w", repository.WrapNetworkError("OpenCode", err))
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenCode request failed: %w", repository.HTTPStatusError("OpenCode", resp.StatusCode))
	}

	fullContent, err := readOpenCodeContent(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), repository.WrapResponseError(p.Name(), err))
	}
	if fullContent == "" {
		return nil, domain.ErrNoResponse
	}

	result, err := repository.DecodeOptionsResponse(p.Name(), fullContent)
	if err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), err)
	}

	return result, nil
}

func readOpenCodeContent(body io.Reader) (string, error) {
	// Читаем streaming-ответ (SSE)
	var fullContent strings.Builder
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), repository.MaxProviderResponseBytes+1)

	for scanner.Scan() {
		line := scanner.Text()

		// SSE формат: "data: {...}" или "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openCodeStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Игнорируем невалидные чанки
			continue
		}

		if len(chunk.Choices) > 0 {
			if err := appendContent(&fullContent, chunk.Choices[0].Delta.Content); err != nil {
				return "", err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading OpenCode stream: %w", err)
	}
	return fullContent.String(), nil
}

func appendContent(content *strings.Builder, value string) error {
	if content.Len()+len(value) > repository.MaxProviderResponseBytes {
		return repository.ErrProviderResponseTooLarge
	}
	content.WriteString(value)
	return nil
}

// CompleteToolTurn uses the non-streaming OpenAI-compatible API so tool
// arguments are returned as complete messages. It never executes tool calls.
func (p *Provider) CompleteToolTurn(ctx context.Context, request domain.ToolRequest) (*domain.ToolTurn, error) {
	config := openaisdk.DefaultConfig(p.apiKey)
	config.BaseURL = p.baseURL
	config.HTTPClient = repository.WithOpenAIToolResponseValidation(p.client)
	turn, err := repository.CompleteOpenAIToolTurn(ctx, repository.OpenAIToolParams{
		Client: openaisdk.NewClientWithConfig(config), Model: p.model,
		ProviderName: p.Name(), MaxCompletionTokens: 32000, Request: request,
	})
	if err != nil {
		return nil, fmt.Errorf("%s tool turn failed: %w", p.Name(), err)
	}
	return turn, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "opencode"
}

// Model returns the configured model identifier.
func (p *Provider) Model() string {
	return p.model
}
