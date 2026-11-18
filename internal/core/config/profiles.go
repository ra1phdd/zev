package config

import (
	"cmp"
	"errors"
	"maps"
)

func legacyProvider(c *Config, providerType string) ProviderConfig {
	if providerType == "" {
		providerType = c.LLMProvider
	}
	switch providerType {
	case kindGemini:
		return ProviderConfig{Type: providerType, APIKey: c.GeminiAPIKey, Model: c.GeminiModel, BaseURL: c.GeminiBaseURL}
	case kindAnthropic:
		return ProviderConfig{Type: providerType, APIKey: c.AnthropicAPIKey, Model: c.AnthropicModel, BaseURL: c.AnthropicBaseURL}
	case kindOllama:
		return ProviderConfig{Type: providerType, Model: c.OllamaModel, BaseURL: c.OllamaBaseURL}
	default:
		return ProviderConfig{Type: providerType, APIKey: c.OpenAIAPIKey, Model: c.OpenAIModel, BaseURL: c.OpenAIBaseURL}
	}
}

func (c *Config) setLegacyProvider(provider ProviderConfig) {
	c.LLMProvider = provider.Type
	switch provider.Type {
	case kindGemini:
		c.GeminiAPIKey, c.GeminiModel, c.GeminiBaseURL = provider.APIKey, provider.Model, provider.BaseURL
	case kindAnthropic:
		c.AnthropicAPIKey, c.AnthropicModel, c.AnthropicBaseURL = provider.APIKey, provider.Model, provider.BaseURL
	case kindOllama:
		c.OllamaModel, c.OllamaBaseURL = provider.Model, provider.BaseURL
	default:
		c.OpenAIAPIKey, c.OpenAIModel, c.OpenAIBaseURL = provider.APIKey, provider.Model, provider.BaseURL
	}
}

// The snapshot records the flat values actually seen by the caller. It is
// immutable; Save applies only the flat-field deltas to a cloned profile map.
type projection struct {
	name, kind string
	byKind     map[string]ProviderConfig
}

var providerSlots = [...]string{kindOpenAI, kindGemini, kindAnthropic, kindOllama}

func (c *Config) captureProjection() {
	s := &projection{name: c.ActiveProvider, kind: c.LLMProvider, byKind: make(map[string]ProviderConfig)}
	for _, kind := range providerSlots {
		s.byKind[kind] = legacyProvider(c, kind)
	}
	c.original = s
}

func (c *Config) project() {
	for _, kind := range providerSlots {
		for _, p := range c.Providers {
			if p.Type == kind {
				c.setLegacyProvider(p)
				break
			}
		}
	}
	if active, ok := c.Providers[c.ActiveProvider]; ok {
		c.setLegacyProvider(active)
	}
	c.captureProjection()
}

func (c Config) forSave() (*Config, error) {
	c.Version = currentConfigVersion
	if c.original == nil && (c.Providers != nil || c.ActiveProvider != "") {
		return &c, c.Validate()
	}
	c.Providers = maps.Clone(c.Providers)
	if c.original == nil && c.Providers == nil && c.ActiveProvider == "" {
		c.LLMProvider = cmp.Or(c.LLMProvider, kindOpenAI)
		c.ActiveProvider = c.LLMProvider
		c.Providers = map[string]ProviderConfig{c.ActiveProvider: legacyProvider(&c, c.LLMProvider)}
	}
	var s projection
	if c.original != nil {
		s = *c.original
	}
	if err := c.applyTypeSwitch(s); err != nil {
		return nil, err
	}
	for _, slot := range providerSlots {
		if err := c.mergeSlot(slot, s.byKind[slot]); err != nil {
			return nil, err
		}
	}
	return &c, c.Validate()
}

// An explicit named selection wins over a legacy type switch. Never replace
// a same-named profile of a different type to accommodate flat fields.
func (c *Config) applyTypeSwitch(s projection) error {
	if c.original == nil || c.ActiveProvider != s.name || c.LLMProvider == s.kind {
		return nil
	}
	if !supported(c.LLMProvider) {
		return errProviderType
	}
	c.ActiveProvider = c.LLMProvider
	if c.Providers == nil {
		return errMissingActive
	}
	if p, ok := c.Providers[c.ActiveProvider]; !ok {
		c.Providers[c.ActiveProvider] = legacyProvider(c, c.LLMProvider)
	} else if p.Type != c.LLMProvider {
		return errors.New("legacy provider switch conflicts with named profile")
	}
	return nil
}

func (c *Config) mergeSlot(slot string, before ProviderConfig) error {
	name, kind := slot, slot
	active := c.Providers[c.ActiveProvider]
	if slot == active.Type || slot == kindOpenAI && active.Type == kindOpenCode {
		name, kind = c.ActiveProvider, active.Type
	}
	now := legacyProvider(c, kind)
	if before.APIKey == now.APIKey && before.Model == now.Model && before.BaseURL == now.BaseURL {
		return nil
	}
	profile, ok := c.Providers[name]
	if !ok {
		if c.Providers == nil || name == c.ActiveProvider {
			return errMissingActive
		}
		profile = now
	}
	if profile.Type != kind {
		return errors.New("flat provider edits conflict with named provider type")
	}
	if before.APIKey != now.APIKey {
		profile.APIKey = now.APIKey
	}
	if before.Model != now.Model {
		profile.Model = now.Model
	}
	if before.BaseURL != now.BaseURL {
		profile.BaseURL = now.BaseURL
	}
	c.Providers[name] = profile
	return nil
}
