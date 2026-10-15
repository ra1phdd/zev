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

// Provider реализует domain.InferenceProvider для Gemini API.
type Provider struct {
	apiKey string
	model  string
	apiURL string
	client *http.Client
}

// New создаёт новый Gemini провайдер.
func New(apiKey, model string) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY is required. Run `zev setup` to configure")
	}
	if model == "" {
		return nil, errors.New("GEMINI_MODEL is required in config")
	}

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	return &Provider{
		apiKey: apiKey,
		model:  model,
		apiURL: apiURL,
		client: &http.Client{},
	}, nil
}

// GetOptions получает варианты команд от Gemini.
func (p *Provider) GetOptions(ctx context.Context, prompt, envContext string) (*domain.OptionsResponse, error) {
	assembledPrompt := fmt.Sprintf(repository.PromptTemplate, envContext, prompt)

	requestBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": assembledPrompt},
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
		return nil, fmt.Errorf("gemini API call failed: %w", err)
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

	var result domain.OptionsResponse
	if err := json.Unmarshal([]byte(textOutput), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// Name возвращает имя провайдера.
func (p *Provider) Name() string {
	return "gemini"
}

// Model возвращает используемую модель.
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
