// Package gemini adapts Gemini APIs to the query provider port.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
)

const (
	geminiJSONParts  = "parts"
	geminiJSONText   = "text"
	geminiJSONType   = "type"
	geminiJSONString = "STRING"
)

// Provider calls Gemini and decodes query options.
type Provider struct {
	apiKey  string
	model   string
	baseURL string
	apiURL  string
	client  *http.Client
}

// New creates a Gemini provider.
func New(apiKey, model, baseURL string, httpClient *http.Client) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY is required. Run `zev setup` to configure")
	}
	if model == "" {
		return nil, errors.New("GEMINI_MODEL is required in config")
	}
	if baseURL == "" {
		return nil, errors.New("GEMINI_BASE_URL is required in config")
	}
	if httpClient == nil {
		return nil, errors.New("httpClient is required")
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, model, apiKey)

	return &Provider{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		apiURL:  apiURL,
		client:  httpClient,
	}, nil
}

// GetOptions requests query options for a new conversation.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	systemPrompt := fmt.Sprintf(repository.SystemPrompt, envContext)
	userMessage := prompt

	requestBody := map[string]any{
		"system_instruction": map[string]any{
			geminiJSONParts: []map[string]string{
				{geminiJSONText: systemPrompt},
			},
		},
		"contents": []map[string]any{
			{
				"role": "user",
				geminiJSONParts: []map[string]string{
					{geminiJSONText: userMessage},
				},
			},
		},
		"generationConfig": geminiResponseSchema,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", repository.WrapNetworkError("Gemini", err))
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini request failed: %w", repository.HTTPStatusError("Gemini", resp.StatusCode))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, domain.ErrNoResponse
	}

	textOutput := geminiResp.Candidates[0].Content.Parts[0].Text
	cleanJSON := repository.ExtractJSON(textOutput)

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), repository.WrapResponseError(p.Name(), err))
	}

	return &result, nil
}

// GetOptionsWithHistory requests query options for an existing conversation.
func (p *Provider) GetOptionsWithHistory(ctx context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	contents := make([]map[string]any, 0, len(history.Messages))

	for _, msg := range history.Messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model" // Gemini использует "model" вместо "assistant"
		}
		contents = append(contents, map[string]any{
			"role": role,
			geminiJSONParts: []map[string]string{
				{geminiJSONText: msg.Content},
			},
		})
	}

	requestBody := map[string]any{
		"system_instruction": map[string]any{
			geminiJSONParts: []map[string]string{
				{geminiJSONText: history.SystemPrompt},
			},
		},
		"contents":         contents,
		"generationConfig": geminiResponseSchema,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", repository.WrapNetworkError("Gemini", err))
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini request failed: %w", repository.HTTPStatusError("Gemini", resp.StatusCode))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, domain.ErrNoResponse
	}

	textOutput := geminiResp.Candidates[0].Content.Parts[0].Text
	cleanJSON := repository.ExtractJSON(textOutput)

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		return nil, fmt.Errorf("%s response failed: %w", p.Name(), repository.WrapResponseError(p.Name(), err))
	}

	return &result, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "gemini"
}

// Model returns the configured model identifier.
func (p *Provider) Model() string {
	return p.model
}

var geminiResponseSchema = map[string]any{
	"response_mime_type": "application/json",
	"response_schema": map[string]any{
		geminiJSONType: "OBJECT",
		"properties": map[string]any{
			"commands": map[string]any{
				geminiJSONType: "ARRAY",
				"items": map[string]any{
					geminiJSONType: "OBJECT",
					"properties": map[string]any{
						"command":               map[string]string{geminiJSONType: geminiJSONString},
						"short_explanation":     map[string]string{geminiJSONType: geminiJSONString},
						"is_dangerous":          map[string]string{geminiJSONType: "BOOLEAN"},
						"dangerous_explanation": map[string]string{geminiJSONType: geminiJSONString},
					},
					"required": []string{"command", "short_explanation", "is_dangerous"},
				},
			},
			"is_valid":                 map[string]string{geminiJSONType: "BOOLEAN"},
			"explanation_if_not_valid": map[string]string{geminiJSONType: geminiJSONString},
		},
		"required": []string{"commands", "is_valid"},
	},
}
