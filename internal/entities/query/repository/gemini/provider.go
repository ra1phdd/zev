package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ra1phdd/zev-go/internal/core/domain"
	"github.com/ra1phdd/zev-go/internal/entities/query/repository"
)

type Provider struct {
	apiKey  string
	model   string
	baseURL string
	apiURL  string
	client  *http.Client
}

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

func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	systemPrompt := fmt.Sprintf(repository.SystemPrompt, envContext)
	userMessage := fmt.Sprintf(repository.UserPromptTemplate, prompt)

	requestBody := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{
				{"text": systemPrompt},
			},
		},
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": userMessage},
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
		return nil, repository.WrapNetworkError("Gemini", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (p *Provider) GetOptionsWithHistory(ctx context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	contents := make([]map[string]any, 0, len(history.Messages))

	for _, msg := range history.Messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model" // Gemini использует "model" вместо "assistant"
		}
		contents = append(contents, map[string]any{
			"role": role,
			"parts": []map[string]string{
				{"text": msg.Content},
			},
		})
	}

	requestBody := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{
				{"text": history.SystemPrompt},
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
		return nil, repository.WrapNetworkError("Gemini", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (p *Provider) Name() string {
	return "gemini"
}

func (p *Provider) Model() string {
	return p.model
}

var geminiResponseSchema = map[string]any{
	"response_mime_type": "application/json",
	"response_schema": map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"commands": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"command":               map[string]string{"type": "STRING"},
						"short_explanation":     map[string]string{"type": "STRING"},
						"is_dangerous":          map[string]string{"type": "BOOLEAN"},
						"dangerous_explanation": map[string]string{"type": "STRING"},
					},
					"required": []string{"command", "short_explanation", "is_dangerous"},
				},
			},
			"is_valid":                 map[string]string{"type": "BOOLEAN"},
			"explanation_if_not_valid": map[string]string{"type": "STRING"},
		},
		"required": []string{"commands", "is_valid"},
	},
}
