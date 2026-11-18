package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

const (
	personalProfile = "personal"
	openAIKind      = "openai"
	anthropicKind   = "anthropic"
	providerWork    = "work"
)

//nolint:paralleltest // Config uses process-wide HOME and the Cobra command tree.
func TestProviderListUsesStableNamesAndMarksActiveProfile(t *testing.T) {
	setupProviderCLIConfig(t, &config.Config{
		ActiveProvider: personalProfile,
		Providers: map[string]config.ProviderConfig{
			providerWork:    {Type: anthropicKind, Model: "work-model"},
			personalProfile: {Type: openAIKind, Model: "personal-model"},
		},
	})

	root := NewRootCommand("test")
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{doctorCheckProvider, "list"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("provider list failed: %v", err)
	}
	if got, want := out.String(), "* personal (openai)\n  work (anthropic)\n"; got != want {
		t.Fatalf("provider list output = %q, want %q", got, want)
	}
}

//nolint:paralleltest // Config uses process-wide HOME and the Cobra command tree.
func TestProviderUseChangesActiveProfileWithoutProjectionWrite(t *testing.T) {
	setupProviderCLIConfig(t, &config.Config{
		ActiveProvider: personalProfile,
		Providers: map[string]config.ProviderConfig{
			providerWork:    {Type: anthropicKind, APIKey: "work-key", Model: "work-model", BaseURL: "https://work.example"},
			personalProfile: {Type: openAIKind, APIKey: "personal-key", Model: "personal-model", BaseURL: "https://personal.example"},
		},
	})

	root := NewRootCommand("test")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	before, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{doctorCheckProvider, "use", providerWork})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("provider use failed: %v", err)
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Providers, before.Providers) || !reflect.DeepEqual(got.HTTPHeaders, before.HTTPHeaders) {
		t.Fatal("switching providers changed stored profiles or headers")
	}
	if got.ActiveProvider != providerWork {
		t.Fatalf("active provider = %q, want work", got.ActiveProvider)
	}
	profile, err := got.ActiveProviderConfig()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Type != anthropicKind || got.LLMProvider != anthropicKind || got.AnthropicAPIKey != "work-key" {
		t.Fatal("active projection did not reflect the selected profile")
	}
}

//nolint:paralleltest // Config uses process-wide HOME and the Cobra command tree.
func TestProviderRemoveRejectsActiveProfileUntilSwitched(t *testing.T) {
	setupProviderCLIConfig(t, &config.Config{
		ActiveProvider: personalProfile,
		Providers: map[string]config.ProviderConfig{
			providerWork:    {Type: anthropicKind},
			personalProfile: {Type: openAIKind},
		},
	})

	root := NewRootCommand("test")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"provider", "remove", personalProfile})
	err := root.ExecuteContext(t.Context())
	if err == nil || !strings.Contains(err.Error(), "active provider") {
		t.Fatalf("remove active error = %v, want active-provider error", err)
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Providers[personalProfile]; !ok {
		t.Fatal("active profile was removed")
	}
}

//nolint:paralleltest // Config uses process-wide HOME and the Cobra command tree.
func TestProviderRemoveRejectsLastActiveProfile(t *testing.T) {
	setupProviderCLIConfig(t, &config.Config{
		ActiveProvider: personalProfile,
		Providers:      map[string]config.ProviderConfig{personalProfile: {Type: openAIKind}},
	})

	root := NewRootCommand("test")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"provider", "remove", personalProfile})
	if err := root.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "active provider") {
		t.Fatalf("remove last provider = %v, want active-provider error", err)
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveProvider != personalProfile || len(got.Providers) != 1 {
		t.Fatalf("after rejected removal = active %q profiles %#v", got.ActiveProvider, got.Providers)
	}
}

//nolint:paralleltest // Uses process-wide HOME and the Cobra command tree.
func TestProviderRemoveInactiveAndRejectMissing(t *testing.T) {
	setupProviderCLIConfig(t, &config.Config{ActiveProvider: personalProfile, Providers: map[string]config.ProviderConfig{
		personalProfile: {Type: openAIKind, Model: "keep"}, providerWork: {Type: openAIKind, Model: managedOpRemove},
	}})
	root := NewRootCommand("test")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{doctorCheckProvider, managedOpRemove, providerWork})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveProvider != personalProfile || len(got.Providers) != 1 || got.Providers[personalProfile].Model != "keep" {
		t.Fatal("expected only the inactive profile to be removed")
	}
	path := filepath.Join(os.Getenv("HOME"), ".zev", "config.yaml")
	before, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // HOME is isolated by the test fixture.
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{managedOpRemove, "use"} {
		root.SetArgs([]string{doctorCheckProvider, command, managedMissingFixture})
		if err := root.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("%s missing profile: %v", command, err)
		}
		after, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // HOME is isolated by the test fixture.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("rejected command modified YAML")
		}
	}
}

func setupProviderCLIConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.Mkdir(filepath.Join(home, ".zev"), 0o700); err != nil {
		t.Fatal(err)
	}
	defaults := config.DefaultConfig()
	defaults.ActiveProvider = cfg.ActiveProvider
	defaults.Providers = cfg.Providers
	defaults.UILanguage = "en"
	if cfg.HTTPHeaders != nil {
		defaults.HTTPHeaders = cfg.HTTPHeaders
	}
	cfg = defaults
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}
