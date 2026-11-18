// Package client provides a generic OpenAI-compatible metadata adapter.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/metadata/service"
)

const (
	// ProviderOpenAICompatible identifies the supported chat-completions protocol.
	ProviderOpenAICompatible = "openai-compatible"
	maxHTTPResponseBytes     = service.MaxGenerationBytes
	providerGemini           = "gemini"
	providerAnthropic        = "anthropic"
	roleUser                 = "user"
)

// Config describes an independent metadata endpoint. APIKey is used only for
// the request Authorization header and is never included in returned errors.
type Config struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
}

// Client implements service.MetadataGenerator over an OpenAI-compatible endpoint.
type Client struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
	provider   string
}

var _ service.MetadataGenerator = (*Client)(nil)

// New validates cfg and returns an adapter using the injected HTTP client.
// The caller owns the client's timeout and transport policy.
func New(cfg Config, httpClient *http.Client) (*Client, error) {
	if httpClient == nil || (cfg.Provider != ProviderOpenAICompatible && cfg.Provider != providerGemini && cfg.Provider != providerAnthropic) || strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("invalid metadata provider configuration")
	}
	baseURL, err := validateBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, errors.New("invalid metadata provider configuration")
	}
	return &Client{baseURL: baseURL, model: cfg.Model, apiKey: cfg.APIKey, httpClient: httpClient, provider: cfg.Provider}, nil
}

// GenerateDescription generates only a description from the bounded input.
func (c *Client) GenerateDescription(ctx context.Context, input service.DescriptionInput) (string, error) {
	prompt, err := service.DescriptionPrompt(input)
	if err != nil {
		return "", fmt.Errorf("build description prompt: %w", err)
	}
	content, err := c.complete(ctx, prompt)
	if err != nil {
		return "", err
	}
	value, err := service.DecodeDescriptionResponse([]byte(content))
	if err != nil {
		return "", fmt.Errorf("decode description response: %w", err)
	}
	return value, nil
}

// GenerateArguments generates only arguments from the bounded input.
func (c *Client) GenerateArguments(ctx context.Context, input service.ArgumentsInput) ([]domain.Argument, error) {
	prompt, err := service.ArgumentsPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("build arguments prompt: %w", err)
	}
	content, err := c.complete(ctx, prompt)
	if err != nil {
		return nil, err
	}
	value, err := service.DecodeArgumentsResponse([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("decode arguments response: %w", err)
	}
	return value, nil
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"` //nolint:tagliatelle // OpenAI-compatible wire format uses snake_case.
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) complete(ctx context.Context, prompt string) (string, error) {
	if ctx == nil {
		return "", service.ErrInvalidInput
	}
	if len([]byte(prompt)) > service.MaxGenerationBytes || !utf8.ValidString(prompt) {
		return "", service.ErrInvalidInput
	}
	payload, path, err := c.requestPayload(prompt)
	if err != nil || len(payload) > service.MaxGenerationBytes {
		return "", service.ErrInvalidInput
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return "", service.ErrInvalidInput
	}
	req.Header.Set("Content-Type", "application/json")
	switch c.provider {
	case providerGemini:
		req.Header.Set("x-goog-api-key", c.apiKey)
	case providerAnthropic:
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("metadata request canceled: %w", ctxErr)
		}
		return "", service.ErrUnavailable
	}
	return c.readCompletionResponse(ctx, resp)
}

func (c *Client) readCompletionResponse(ctx context.Context, resp *http.Response) (string, error) {
	if resp == nil || resp.Body == nil {
		return "", service.ErrUnavailable
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if err := resp.Body.Close(); err != nil {
			return "", service.ErrUnavailable
		}
		return "", service.ErrUnavailable
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseBytes+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("metadata response canceled: %w", ctxErr)
		}
		return "", service.ErrUnavailable
	}
	if closeErr != nil || len(body) > maxHTTPResponseBytes {
		if closeErr != nil {
			return "", service.ErrUnavailable
		}
		return "", service.ErrInvalidResponse
	}
	if !utf8.Valid(body) {
		return "", service.ErrInvalidResponse
	}
	return c.responseContent(body)
}

func (c *Client) requestPayload(prompt string) ([]byte, string, error) {
	var payload any
	var path string
	switch c.provider {
	case providerGemini:
		payload = map[string]any{"contents": []any{map[string]any{"role": roleUser, "parts": []any{map[string]string{"text": prompt}}}}}
		path = "/v1beta/models/" + c.model + ":generateContent"
	case providerAnthropic:
		payload = map[string]any{"model": c.model, "max_tokens": 2048, "system": "You generate managed metadata only.", "messages": []any{map[string]string{"role": roleUser, "content": prompt}}}
		path = "/messages"
	default:
		payload = chatRequest{Model: c.model, Messages: []chatMessage{{Role: "system", Content: "You generate managed alias and script metadata. Do not use outside context."}, {Role: roleUser, Content: prompt}}, ResponseFormat: responseFormat{Type: "json_object"}}
		path = "/chat/completions"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("encode metadata request: %w", err)
	}
	return body, path, nil
}

func (c *Client) responseContent(body []byte) (string, error) {
	if err := service.ValidateResponseDocument(body); err != nil {
		return "", service.ErrInvalidResponse
	}
	switch c.provider {
	case providerGemini:
		var response struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &response); err != nil || len(response.Candidates) != 1 || len(response.Candidates[0].Content.Parts) != 1 {
			return "", service.ErrInvalidResponse
		}
		return response.Candidates[0].Content.Parts[0].Text, nil
	case providerAnthropic:
		var response struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &response); err != nil || len(response.Content) != 1 || response.Content[0].Type != "text" {
			return "", service.ErrInvalidResponse
		}
		return response.Content[0].Text, nil
	default:
		var response chatResponse
		if err := json.Unmarshal(body, &response); err != nil || len(response.Choices) != 1 || response.Choices[0].Message.Content == "" {
			return "", service.ErrInvalidResponse
		}
		return response.Choices[0].Message.Content, nil
	}
}

func validateBaseURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("empty endpoint")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("invalid endpoint")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
