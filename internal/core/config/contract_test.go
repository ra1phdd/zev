package config

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestStrictConfigDocument(t *testing.T) {
	t.Parallel()
	const secret = "private-fixture"
	for name, data := range map[string]string{
		"missing active":        "version: 1\nproviders: {work: {type: gemini}}",
		"dangling active":       "version: 1\nactive_provider: work\nproviders: {openai: {type: openai}}",
		"unknown field":         minimalYAML + secret + ": value",
		"unknown profile field": "version: 1\nactive_provider: work\nproviders: {work: {type: ollama, bad: " + secret + "}}",
		"version":               minimalYAML + "version: 43",
		"extra document":        minimalYAML + "---\n",
		"duplicate":             minimalYAML + "active_provider: " + secret,
		"bad type":              minimalYAML + "history_enabled: {}",
		"null boolean":          minimalYAML + "tools_enabled: null",
		"invalid provider":      "version: 1\nactive_provider: work\nproviders: {work: {type: " + secret + "}}",
		"nul header value":      minimalYAML + "http_headers: {X-Token: null}",
		"dotenv":                "OPENAI_MODEL=" + secret,
		"invalid profile model": "version: 1\nactive_provider: work\nproviders: {work: {type: openai, model: false}}",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(data)); err == nil || strings.Contains(err.Error(), secret) {
				t.Fatalf("missing or unsafe diagnostic: %v", err)
			}
		})
	}
}

func TestProjectionMergesOnlyFlatEdits(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]byte(`version: 1
active_provider: work
providers:
  work: {type: openai, api_key: initial, model: initial, http_headers: {X-Profile: work}}
  other: {type: openai, api_key: independent}
http_headers: {x-profile: global, X-Common: common}
`))
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Providers[workProfile]
	p.Model = "direct"
	cfg.Providers[workProfile] = p
	cfg.OpenAIAPIKey = "flat"
	data, err := encodeYAML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.OpenAIModel != "direct" || got.OpenAIAPIKey != "flat" || got.Providers["other"].APIKey != "independent" || cfg.Providers[workProfile].APIKey != "initial" {
		t.Fatal("flat projection overwrote independent state")
	}
	active, err := got.ActiveProviderConfig()
	if err != nil {
		t.Fatal(err)
	}
	active.HTTPHeaders["X-Profile"] = "detached"
	if !maps.Equal(got.EffectiveHTTPHeaders(), map[string]string{"X-Profile": workProfile, "X-Common": "common"}) {
		t.Fatal("wrong effective headers")
	}
	got.ActiveProvider = "other"
	if got.EffectiveHTTPHeaders()["X-Profile"] != "global" {
		t.Fatal("inactive headers leaked")
	}
	if !slices.Equal(got.ProviderNames(), []string{"other", workProfile}) {
		t.Fatal("unexpected profile order")
	}
}

func TestSaveRejectsMissingActiveWithoutOverwritingProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ActiveProvider = "missing"
	if err := Save(cfg); !errors.Is(err, errMissingActive) {
		t.Fatalf("expected missing active error; got %v", err)
	}
	got, err := Load()
	if err != nil || got.ActiveProvider != kindOpenAI {
		t.Fatal("invalid save changed persisted selection")
	}
}
