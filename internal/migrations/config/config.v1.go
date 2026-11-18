// Package config migrates settings written by released versions of Zev.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	current "github.com/ra1phdd/zev/internal/core/config"
	"gopkg.in/yaml.v3"
)

// dotenvToV1 converts the fields persisted by v1.0.0's config.Save. It does
// not consult the environment or emit an intermediate document.
func dotenvToV1(data []byte) ([]byte, error) {
	values, err := godotenv.UnmarshalBytes(data)
	if err != nil {
		return nil, errors.New("invalid dotenv configuration")
	}
	cfg := current.DefaultConfig()
	cfg.Version = 1
	take := func(key string, fallback string) string {
		value, ok := values[key]
		delete(values, key)
		if ok {
			return value
		}
		return fallback
	}
	cfg.ActiveProvider = take("LLM_PROVIDER", cfg.ActiveProvider)
	cfg.ScriptEditor = take("SCRIPT_EDITOR", cfg.ScriptEditor)
	cfg.CommandExecutionMode = take("COMMAND_EXECUTION_MODE", cfg.CommandExecutionMode)
	if value, ok := values["HTTP_HEADERS"]; ok {
		cfg.HTTPHeaders, err = parseHTTPHeaders(value)
		if err != nil {
			return nil, err
		}
		delete(values, "HTTP_HEADERS")
	}
	cfg.Providers = make(map[string]current.ProviderConfig)
	for _, p := range []current.ProviderConfig{
		{Type: "openai", Model: cfg.OpenAIModel, BaseURL: cfg.OpenAIBaseURL},
		{Type: "gemini", Model: cfg.GeminiModel, BaseURL: cfg.GeminiBaseURL},
		{Type: "anthropic", Model: cfg.AnthropicModel, BaseURL: cfg.AnthropicBaseURL},
		{Type: "ollama", Model: cfg.OllamaModel, BaseURL: cfg.OllamaBaseURL},
	} {
		prefix := strings.ToUpper(p.Type) + "_"
		before := len(values)
		if p.Type != "ollama" {
			p.APIKey = take(prefix+"API_KEY", "")
		}
		p.Model = take(prefix+"MODEL", p.Model)
		p.BaseURL = take(prefix+"BASE_URL", p.BaseURL)
		if p.Type == cfg.ActiveProvider || len(values) != before {
			cfg.Providers[p.Type] = p
		}
		if cfg.ActiveProvider == "opencode" && p.Type == "openai" {
			p.Type = "opencode"
			p.HTTPHeaders = cfg.HTTPHeaders
			cfg.Providers[p.Type] = p
			cfg.HTTPHeaders = map[string]string{}
		}
	}
	if len(values) != 0 {
		return nil, errors.New("unsupported dotenv setting; source preserved")
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, errors.New("cannot encode converted configuration")
	}
	if _, err := current.Parse(out); err != nil {
		return nil, fmt.Errorf("validate converted configuration: %w", err)
	}
	return out, nil
}

func parseHTTPHeaders(input string) (map[string]string, error) {
	headers := make(map[string]string)
	if strings.TrimSpace(input) == "" {
		return headers, nil
	}
	for pair := range strings.SplitSeq(input, ",") {
		key, value, ok := strings.Cut(pair, ":")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, errors.New("invalid legacy HTTP headers")
		}
		headers[key] = strings.TrimSpace(value)
	}
	return headers, nil
}
