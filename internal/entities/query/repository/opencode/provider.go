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
	Model               string                  `json:"model"`
	Messages            []openCodeMessage       `json:"messages"`
	Tools               []openCodeTool          `json:"tools"`
	Stream              bool                    `json:"stream"`
	StreamOptions       openCodeStreamOptions   `json:"stream_options"`
	MaxCompletionTokens int                     `json:"max_completion_tokens"`
	ResponseFormat      *openCodeResponseFormat `json:"response_format,omitempty"`
}

type openCodeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openCodeTool struct {
	Type     string               `json:"type"`
	Function openCodeToolFunction `json:"function"`
}

type openCodeToolFunction struct {
	Name string `json:"name"`
}

type openCodeStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
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
		FinishReason *string `json:"finish_reason"`
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
		Tools: []openCodeTool{
			{
				Type: "function",
				Function: openCodeToolFunction{
					Name: "read",
				},
			},
		},
		Stream:              true,
		StreamOptions:       openCodeStreamOptions{IncludeUsage: true},
		MaxCompletionTokens: 32000,
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

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Host", "opencode.ai")
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

	// Читаем streaming-ответ (SSE)
	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

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
			fullContent.WriteString(chunk.Choices[0].Delta.Content)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read streaming response: %w", err)
	}

	if fullContent.Len() == 0 {
		return nil, domain.ErrNoResponse
	}

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(fullContent.String()), &result); err != nil {
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
