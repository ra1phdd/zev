package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	"gopkg.in/yaml.v3"
)

const (
	cliFixtureModel        = "fixture-model"
	cliFixtureType         = "type"
	cliFixtureInvalidValue = "invalid"
)

func newCLIConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func openAITestConfig(baseURL, key string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.ActiveProvider = "fixture"
	cfg.Providers = map[string]config.ProviderConfig{"fixture": {Type: "openai", APIKey: key, Model: cliFixtureModel, BaseURL: baseURL}}
	return cfg
}

func invalidProviderFixture() *config.Config {
	cfg := config.DefaultConfig()
	cfg.ActiveProvider = cliFixtureInvalidValue
	cfg.Providers = map[string]config.ProviderConfig{cliFixtureInvalidValue: {Type: cliFixtureInvalidValue}}
	cfg.ToolApprovalMode = cliFixtureInvalidValue
	return cfg
}

// Local-only commands must also work with configurations that Save would reject.
func writeCLIConfig(t *testing.T, home string, settings any) {
	t.Helper()
	data, err := yaml.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// These presentation tests construct subcommands directly rather than going through Execute.
func setFixtureLanguage(t *testing.T, home, language string) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.UILanguage = language
	writeCLIConfig(t, home, cfg)
	selected, err := config.LoadUILanguage()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(locale.SetLanguage(selected, ""))
}
