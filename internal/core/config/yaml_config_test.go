package config

import (
	"path/filepath"
	"testing"
)

//nolint:paralleltest // Mutates the global config path.
func TestYAMLConfigRoundTripsMultipleProvidersAndActiveSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFilename)
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })

	cfg := DefaultConfig()
	cfg.ActiveProvider = workProfile
	cfg.Providers = map[string]ProviderConfig{
		workProfile: {Type: kindOpenAI, APIKey: "work-secret", Model: "work-model", BaseURL: "https://work.example/v1"},
		"local":     {Type: kindOllama, Model: "local-model", BaseURL: "http://localhost:11434/v1"},
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	active, err := got.ActiveProviderConfig()
	if err != nil {
		t.Fatal(err)
	}
	if active.Type != kindOpenAI || active.APIKey != "work-secret" || active.Model != "work-model" {
		t.Fatalf("active provider = %+v", active)
	}
	if len(got.ProviderNames()) != 2 {
		t.Fatalf("provider names = %#v", got.ProviderNames())
	}
}

func TestLoadDoesNotUseEnvironmentConfigurationOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFilename)
	previous := SetConfigPathForTests(path)
	t.Cleanup(func() { SetConfigPathForTests(previous) })
	t.Setenv("OPENAI_MODEL", "environment-model")

	cfg := DefaultConfig()
	cfg.OpenAIModel = "yaml-model"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.OpenAIModel != "yaml-model" {
		t.Fatalf("model = %q, want yaml-model", got.OpenAIModel)
	}
}

const workProfile = "work"
