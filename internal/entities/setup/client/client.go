// Package client lists models exposed by configured LLM providers.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Provider identifiers supported by the model catalog client.
const (
	ProviderOpenAI    = "openai"
	ProviderGemini    = "gemini"
	ProviderAnthropic = "anthropic"
	ProviderOllama    = "ollama"
	ProviderOpenCode  = "opencode"

	maxResponseBytes = 1 << 20
	maxModelPages    = 1000
)

// Request contains the provider connection settings needed to list models.
type Request struct {
	Provider string
	APIKey   string
	BaseURL  string
}

// Client retrieves provider model catalogs using the supplied HTTP client.
type Client struct {
	httpClient *http.Client
}

// New creates a model-list client.
func New(httpClient *http.Client) *Client {
	return &Client{httpClient: httpClient}
}

// ListModels returns model identifiers available to the configured credentials.
// Providers that expose capabilities are filtered to models usable for text
// generation by Zev.
func (c *Client) ListModels(ctx context.Context, request Request) ([]string, error) {
	if c == nil || c.httpClient == nil {
		return nil, errors.New("http client is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("check model-list context: %w", err)
	}

	switch request.Provider {
	case ProviderOpenAI:
		return c.listOpenAI(ctx, request)
	case ProviderGemini:
		return c.listGemini(ctx, request)
	case ProviderAnthropic:
		return c.listAnthropic(ctx, request)
	case ProviderOllama:
		return c.listOllama(ctx, request)
	case ProviderOpenCode:
		return c.listOpenCode(ctx, request)
	default:
		return nil, fmt.Errorf("unsupported provider %q", request.Provider)
	}
}

func (c *Client) listOpenAI(ctx context.Context, request Request) ([]string, error) {
	var response openAIModelsResponse
	if err := c.getJSON(ctx, appendPath(request.BaseURL, "/models"), request.APIKey, "", &response); err != nil {
		return nil, fmt.Errorf("list OpenAI models: %w", err)
	}
	return normalize(response.IDs()), nil
}

func (c *Client) listOpenCode(ctx context.Context, request Request) ([]string, error) {
	var response openAIModelsResponse
	if err := c.getJSON(ctx, openCodeModelsURL(request.BaseURL), request.APIKey, "", &response); err != nil {
		return nil, fmt.Errorf("list OpenCode models: %w", err)
	}
	return normalize(response.IDs()), nil
}

func (c *Client) listGemini(ctx context.Context, request Request) ([]string, error) {
	var models []string
	pageToken := ""
	for page := 0; ; page++ {
		if page >= maxModelPages {
			return nil, errors.New("model pagination exceeded the safety limit for Gemini")
		}
		endpoint, err := queryURL(appendPath(request.BaseURL, "/v1beta/models"), "pageSize", "1000")
		if err != nil {
			return nil, fmt.Errorf("build Gemini models URL: %w", err)
		}
		if pageToken != "" {
			endpoint, err = queryURL(endpoint, "pageToken", pageToken)
			if err != nil {
				return nil, fmt.Errorf("build Gemini models page URL: %w", err)
			}
		}

		var response geminiModelsResponse
		if err := c.getJSON(ctx, endpoint, request.APIKey, "x-goog-api-key", &response); err != nil {
			return nil, fmt.Errorf("list Gemini models: %w", err)
		}
		for _, model := range response.Models {
			if !slices.Contains(model.SupportedGenerationMethods, "generateContent") {
				continue
			}
			models = append(models, strings.TrimPrefix(model.Name, "models/"))
		}
		if response.NextPageToken == "" {
			break
		}
		pageToken = response.NextPageToken
	}
	return normalize(models), nil
}

func (c *Client) listAnthropic(ctx context.Context, request Request) ([]string, error) {
	var models []string
	afterID := ""
	for page := 0; ; page++ {
		if page >= maxModelPages {
			return nil, errors.New("model pagination exceeded the safety limit for Anthropic")
		}
		endpoint, err := queryURL(appendPath(request.BaseURL, "/models"), "limit", "1000")
		if err != nil {
			return nil, fmt.Errorf("build Anthropic models URL: %w", err)
		}
		if afterID != "" {
			endpoint, err = queryURL(endpoint, "after_id", afterID)
			if err != nil {
				return nil, fmt.Errorf("build Anthropic models page URL: %w", err)
			}
		}

		var response anthropicModelsResponse
		if err := c.getJSON(ctx, endpoint, request.APIKey, "x-api-key", &response); err != nil {
			return nil, fmt.Errorf("list Anthropic models: %w", err)
		}
		for _, model := range response.Data {
			models = append(models, model.ID)
		}
		if !response.HasMore || response.LastID == "" {
			break
		}
		afterID = response.LastID
	}
	return normalize(models), nil
}

func (c *Client) listOllama(ctx context.Context, request Request) ([]string, error) {
	var response ollamaModelsResponse
	if err := c.getJSON(ctx, ollamaModelsURL(request.BaseURL), "", "", &response); err != nil {
		return nil, fmt.Errorf("list Ollama models: %w", err)
	}
	models := make([]string, 0, len(response.Models))
	for _, model := range response.Models {
		if model.Name != "" {
			models = append(models, model.Name)
			continue
		}
		models = append(models, model.Model)
	}
	return normalize(models), nil
}

func (c *Client) getJSON(ctx context.Context, endpoint, apiKey, apiKeyHeader string, target any) (err error) {
	if err := validateEndpoint(endpoint); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if apiKey != "" {
		if apiKeyHeader == "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else {
			req.Header.Set(apiKeyHeader, apiKey)
		}
	}
	if apiKeyHeader == "x-api-key" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close provider response: %w", closeErr))
		}
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("request returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return errors.New("response is too large")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func validateEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("provider base URL must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("provider base URL must use HTTP or HTTPS")
	}
	return nil
}

func appendPath(base, suffix string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return strings.TrimRight(base, "/") + suffix
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + suffix
	return parsed.String()
}

func openCodeModelsURL(base string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return strings.TrimRight(base, "/") + "/models"
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/inference/openai/v1") {
		path = strings.TrimSuffix(path, "/openai/v1") + "/v1/models"
	} else {
		path += "/models"
	}
	parsed.Path = path
	return parsed.String()
}

func ollamaModelsURL(base string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return strings.TrimRight(base, "/") + "/api/tags"
	}
	path := strings.TrimRight(parsed.Path, "/")
	if before, ok := strings.CutSuffix(path, "/v1"); ok {
		path = before
	}
	parsed.Path = path + "/api/tags"
	return parsed.String()
}

func queryURL(endpoint, key, value string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse provider URL: %w", err)
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func normalize(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	slices.Sort(result)
	return result
}

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (r openAIModelsResponse) IDs() []string {
	models := make([]string, 0, len(r.Data))
	for _, model := range r.Data {
		models = append(models, model.ID)
	}
	return models
}

type geminiModelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
	NextPageToken string `json:"nextPageToken"`
}

type anthropicModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	HasMore bool   `json:"has_more"` //nolint:tagliatelle // Provider wire format uses snake_case.
	LastID  string `json:"last_id"`  //nolint:tagliatelle // Provider wire format uses snake_case.
}

type ollamaModelsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}
