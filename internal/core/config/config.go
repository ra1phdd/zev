// Package config loads and saves Zev settings in a local YAML file.
package config

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"unicode"
)

const currentConfigVersion = 1

// ProviderConfig stores one named LLM provider profile.
//
//nolint:tagliatelle // The YAML wire format uses snake_case.
type ProviderConfig struct {
	Type        string            `yaml:"type"`
	APIKey      string            `yaml:"api_key,omitempty"`
	Model       string            `yaml:"model"`
	BaseURL     string            `yaml:"base_url"`
	HTTPHeaders map[string]string `yaml:"http_headers,omitempty"`
}

// Config contains provider, HTTP, and command execution settings.
//
//nolint:tagliatelle // The YAML wire format uses snake_case.
type Config struct {
	original *projection

	Version        int                       `yaml:"version"`
	ActiveProvider string                    `yaml:"active_provider"`
	Providers      map[string]ProviderConfig `yaml:"providers"`
	UILanguage     string                    `yaml:"language"`
	HTTPHeaders    map[string]string         `yaml:"http_headers"`

	// The flattened provider fields remain an in-memory compatibility projection
	// for existing adapters. They are never written to YAML.
	LLMProvider string `yaml:"-"`

	OpenAIAPIKey  string `yaml:"-"`
	OpenAIModel   string `yaml:"-"`
	OpenAIBaseURL string `yaml:"-"`
	GeminiAPIKey  string `yaml:"-"`
	GeminiModel   string `yaml:"-"`
	GeminiBaseURL string `yaml:"-"`

	AnthropicAPIKey  string `yaml:"-"`
	AnthropicModel   string `yaml:"-"`
	AnthropicBaseURL string `yaml:"-"`

	OllamaBaseURL string `yaml:"-"`
	OllamaModel   string `yaml:"-"`

	CommandExecutionMode string `yaml:"command_execution_mode"`
	ScriptEditor         string `yaml:"script_editor"`
	HistoryEnabled       bool   `yaml:"history_enabled"`
	ContextEnabled       bool   `yaml:"context_enabled"`
	ToolsEnabled         bool   `yaml:"tools_enabled"`
	ToolApprovalMode     string `yaml:"tool_approval_mode"`

	HistorySemanticEnabled     bool   `yaml:"history_semantic_enabled"`
	HistoryEmbeddingProvider   string `yaml:"history_embedding_provider,omitempty"`
	HistoryEmbeddingBaseURL    string `yaml:"history_embedding_base_url,omitempty"`
	HistoryEmbeddingModel      string `yaml:"history_embedding_model,omitempty"`
	HistoryEmbeddingDimensions int    `yaml:"history_embedding_dimensions,omitempty"`
	HistoryEmbeddingAPIKey     string `yaml:"history_embedding_api_key,omitempty"`
	HistoryEmbeddingMaxBytes   int    `yaml:"history_embedding_max_bytes"`
}

// DefaultConfig returns a new configuration with default provider and CLI settings.
func DefaultConfig() *Config {
	cfg := &Config{
		Version:        currentConfigVersion,
		ActiveProvider: kindOpenAI,
		Providers:      make(map[string]ProviderConfig),
		UILanguage:     uiLanguageAuto,
		HTTPHeaders: map[string]string{
			"User-Agent": "zev/1.0",
		},
		OpenAIModel:              "gpt-5.6-luna",
		OpenAIBaseURL:            "https://api.openai.com/v1",
		GeminiModel:              "gemini-3.1-flash-lite",
		GeminiBaseURL:            "https://generativelanguage.googleapis.com",
		AnthropicModel:           "claude-haiku-5.5",
		AnthropicBaseURL:         "https://api.anthropic.com/v1",
		CommandExecutionMode:     "inject",
		ScriptEditor:             "nano",
		HistoryEnabled:           true,
		ContextEnabled:           true,
		ToolsEnabled:             true,
		ToolApprovalMode:         "manual",
		HistoryEmbeddingMaxBytes: 32768,
	}
	cfg.LLMProvider = cfg.ActiveProvider
	cfg.Providers[cfg.ActiveProvider] = legacyProvider(cfg, cfg.ActiveProvider)
	cfg.captureProjection()
	return cfg
}

// ProviderNames returns the configured names in lexical order.
func (c *Config) ProviderNames() []string {
	if c == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(c.Providers))
}

// ActiveProviderConfig returns the selected profile with a detached header map.
func (c *Config) ActiveProviderConfig() (ProviderConfig, error) {
	if c == nil {
		return ProviderConfig{}, errMissingActive
	}
	p, ok := c.Providers[c.ActiveProvider]
	if !ok || c.ActiveProvider == "" {
		return ProviderConfig{}, errMissingActive
	}
	if !supported(p.Type) {
		return ProviderConfig{}, errProviderType
	}
	p.HTTPHeaders = maps.Clone(p.HTTPHeaders)
	return p, nil
}

// EffectiveHTTPHeaders returns global request headers overridden by the active
// profile's headers. Neither configuration map is modified.
func (c *Config) EffectiveHTTPHeaders() map[string]string {
	headers := make(map[string]string)
	for _, m := range []map[string]string{c.HTTPHeaders, c.Providers[c.ActiveProvider].HTTPHeaders} {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			headers[http.CanonicalHeaderKey(k)] = m[k]
		}
	}
	return headers
}

var (
	errMissingActive     = errors.New("active provider is not configured")
	errProviderType      = errors.New("unsupported provider type")
	errInvalidToolPolicy = errors.New("invalid tool policy")
	errInvalidUILanguage = errors.New("invalid UI language: must be auto, en, or ru")
)

func supported(kind string) bool {
	switch kind {
	case kindOpenAI, kindGemini, kindAnthropic, kindOllama, kindOpenCode:
		return true
	}
	return false
}

// Validate checks UI language, tool policy, and opt-in history embedding settings.
// Empty UI language means auto. Disabled semantic search deliberately ignores the
// other embedding fields. Invalid UI language errors never include its value.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if _, err := c.ActiveProviderConfig(); err != nil {
		return err
	}
	for name, p := range c.Providers {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || strings.ContainsFunc(name, unicode.IsControl) {
			return errors.New("invalid provider name")
		}
		if !supported(p.Type) {
			return errProviderType
		}
	}
	c.UILanguage = normalizeUILanguage(c.UILanguage)
	if err := validateUILanguage(c.UILanguage); err != nil {
		return err
	}
	if c.ToolApprovalMode != "manual" && c.ToolApprovalMode != "auto" && c.ToolApprovalMode != "bypass" {
		return fmt.Errorf("%w: approval mode must be manual, auto, or bypass", errInvalidToolPolicy)
	}
	if !c.HistorySemanticEnabled {
		return nil
	}
	if c.HistoryEmbeddingProvider != "openai-compatible" {
		return errors.New("unsupported history embedding provider")
	}
	if strings.TrimSpace(c.HistoryEmbeddingBaseURL) == "" {
		return errors.New("history embedding base URL is required")
	}
	if strings.TrimSpace(c.HistoryEmbeddingModel) == "" {
		return errors.New("history embedding model is required")
	}
	if c.HistoryEmbeddingDimensions <= 0 {
		return errors.New("history embedding dimensions must be positive")
	}
	if c.HistoryEmbeddingMaxBytes <= 0 {
		return errors.New("history embedding max bytes must be positive")
	}
	return nil
}

const (
	kindOpenAI    = "openai"
	kindOpenCode  = "opencode"
	kindGemini    = "gemini"
	kindOllama    = "ollama"
	kindAnthropic = "anthropic"
)
