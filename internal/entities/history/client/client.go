// Package client provides the opt-in OpenAI-compatible history embedding adapter.
package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// ProviderOpenAICompatible is the only embedding provider supported here.
	ProviderOpenAICompatible = "openai-compatible"
	maxResponseBytes         = 1 << 20
	profilePreprocessing     = "utf8-exact-v1"
	profileMetric            = "cosine"
	profileEncoding          = "le-f32"
)

var (
	errInvalidConfig = errors.New("invalid embedding configuration")
	errInvalidInput  = errors.New("invalid embedding input")

	// ErrUnavailable classifies provider transport and response failures.
	ErrUnavailable = errors.New("embedding unavailable")
)

// Config describes one independent embedding provider configuration. It does
// not inherit command LLM credentials or headers.
type Config struct {
	Provider   string
	BaseURL    string
	Model      string
	APIKey     string
	Dimensions int
	MaxBytes   int
}

// Client sends embedding requests to an OpenAI-compatible endpoint.
type Client struct {
	provider   string
	baseURL    string
	model      string
	apiKey     string
	dimensions int
	maxBytes   int
	httpClient *http.Client
	profileID  string
}

// New validates cfg and returns a client using the supplied shared HTTP client.
// The HTTP client must be non-nil and have a positive timeout so provider I/O is bounded.
func New(cfg Config, httpClient *http.Client) (*Client, error) {
	if httpClient == nil || httpClient.Timeout <= 0 {
		return nil, errInvalidConfig
	}
	if cfg.Provider != ProviderOpenAICompatible || strings.TrimSpace(cfg.Model) == "" || cfg.Dimensions <= 0 || cfg.MaxBytes <= 0 {
		return nil, errInvalidConfig
	}
	baseURL, err := validateBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, errInvalidConfig
	}

	profileInput := strings.Join([]string{
		cfg.Provider,
		baseURL,
		cfg.Model,
		strconv.Itoa(cfg.Dimensions),
		profilePreprocessing,
		profileMetric,
		profileEncoding,
	}, "\x00")
	hash := sha256.Sum256([]byte(profileInput))

	return &Client{
		provider:   cfg.Provider,
		baseURL:    baseURL,
		model:      cfg.Model,
		apiKey:     cfg.APIKey,
		dimensions: cfg.Dimensions,
		maxBytes:   cfg.MaxBytes,
		httpClient: httpClient,
		profileID:  hex.EncodeToString(hash[:]),
	}, nil
}

// Embed requests one vector for text. Input is measured in UTF-8 bytes and is
// rejected when it exceeds MaxBytes; it is never truncated. ctx must be non-nil.
// Provider failures return ErrUnavailable without response or request details.
// Caller cancellation and deadlines wrap ctx.Err() for errors.Is classification.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if ctx == nil {
		return nil, errInvalidInput
	}
	if !utf8.ValidString(text) || len(text) > c.maxBytes {
		return nil, errInvalidInput
	}

	payload, err := json.Marshal(struct {
		Model      string `json:"model"`
		Input      string `json:"input"`
		Dimensions int    `json:"dimensions"`
	}{
		Model:      c.model,
		Input:      text,
		Dimensions: c.dimensions,
	})
	if err != nil {
		return nil, errInvalidInput
	}
	requestURL := c.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return nil, errInvalidInput
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("embedding request canceled: %w", ctxErr)
		}
		return nil, ErrUnavailable
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if err := resp.Body.Close(); err != nil {
			return nil, ErrUnavailable
		}
		return nil, ErrUnavailable
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("embedding response canceled: %w", ctxErr)
		}
		return nil, ErrUnavailable
	}
	if closeErr != nil || len(body) > maxResponseBytes {
		return nil, ErrUnavailable
	}
	return decodeVector(body, c.dimensions)
}

// ProfileID identifies the vector representation and comparison profile. It
// excludes the API key and request-size limit so credentials and transport
// policy changes do not split compatible vectors.
func (c *Client) ProfileID() string { return c.profileID }

// Model returns the configured embedding model.
func (c *Client) Model() string { return c.model }

// Dimensions returns the configured vector dimensionality.
func (c *Client) Dimensions() int { return c.dimensions }

// MaxBytes returns the maximum UTF-8 input size accepted by Embed.
func (c *Client) MaxBytes() int { return c.maxBytes }

func validateBaseURL(raw string) (string, error) {
	if raw == "" {
		return "", errInvalidConfig
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", errInvalidConfig
	}
	return strings.TrimRight(u.String(), "/"), nil
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func decodeVector(body []byte, dimensions int) ([]float32, error) {
	var response embeddingResponse
	if err := json.Unmarshal(body, &response); err != nil || len(response.Data) != 1 || len(response.Data[0].Embedding) != dimensions {
		return nil, ErrUnavailable
	}

	vector := make([]float32, dimensions)
	norm := 0.0
	for i, value := range response.Data[0].Embedding {
		converted := float32(value)
		if math.IsNaN(float64(converted)) || math.IsInf(float64(converted), 0) {
			return nil, ErrUnavailable
		}
		vector[i] = converted
		norm += float64(converted) * float64(converted)
	}
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return nil, ErrUnavailable
	}
	return vector, nil
}
