// Package service implements the interactive configuration setup flow.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/ra1phdd/zev/pkg/tui/prompt"
)

const (
	providerOpenAI    = "openai"
	providerGemini    = "gemini"
	providerAnthropic = "anthropic"
	providerOllama    = "ollama"
	providerOpenCode  = "opencode"
)

// Service runs the interactive configuration setup flow.
type Service struct {
	runSecretInput func(tea.Model) (tea.Model, error)
	inputContext   func(context.Context, string) (string, error)
	selectContext  func(context.Context, list.SelectOptions) (list.Item, error)
	listModels     modelFetcher
	warn           func(string)
	proxyURL       string
}

// New creates a setup service.
func New() *Service {
	return NewWithProxy("")
}

// NewWithProxy creates a setup service that routes model discovery through the
// supplied proxy, or through the configured environment default when empty.
func NewWithProxy(proxyURL string) *Service {
	return &Service{
		inputContext:  prompt.InputContext,
		selectContext: list.SelectContext,
		listModels:    defaultModelFetcher,
		warn:          func(message string) { _, _ = fmt.Fprintln(os.Stderr, message) },
		proxyURL:      proxyURL,
	}
}

// maskAPIKey conceals the entire saved key, including its length.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	return "***"
}

// promptWithDefault preserves current on Enter and accepts the literal "default"
// to restore defaultVal. Other input is returned unchanged.
func (s *Service) promptWithDefault(ctx context.Context, promptKey locale.Key, current, defaultVal string) (string, error) {
	promptText := locale.Text(promptKey)
	var fullPrompt string
	switch {
	case current != "" && defaultVal != "" && current != defaultVal:
		fullPrompt = fmt.Sprintf(locale.Text(locale.KeyUISetupCurrentDefault), promptText, current, defaultVal)
	case current != "":
		fullPrompt = fmt.Sprintf(locale.Text(locale.KeyUISetupCurrent), promptText, current)
	case defaultVal != "":
		fullPrompt = fmt.Sprintf(locale.Text(locale.KeyUISetupDefault), promptText, defaultVal)
	default:
		fullPrompt = promptText + ":"
	}

	input, err := s.inputContext(ctx, fullPrompt)
	if err != nil {
		return "", fmt.Errorf("read setup input: %w", err)
	}

	if input == "" {
		if current != "" {
			return current, nil
		}
		return defaultVal, nil
	}

	if input == "default" && defaultVal != "" {
		return defaultVal, nil
	}

	return input, nil
}

// RunContext prompts for provider settings and saves them unless ctx is canceled.
func (s *Service) RunContext(ctx context.Context) error {
	return s.runSetup(ctx, "")
}

// RunNamedContext adds a provider profile without changing the active profile.
func (s *Service) RunNamedContext(ctx context.Context, name string) error {
	if name == "" || name != strings.TrimSpace(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New("invalid provider name")
	}
	return s.runSetup(ctx, name)
}

func (s Service) runSetup(ctx context.Context, newName string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("setup canceled: %w", err)
	}
	if s.inputContext == nil {
		s.inputContext = prompt.InputContext
	}
	if s.selectContext == nil {
		s.selectContext = list.SelectContext
	}
	original, err := config.LoadContext(ctx)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if _, exists := original.Providers[newName]; exists {
		return fmt.Errorf(locale.Text(locale.KeyProviderAlreadyExists), newName)
	}
	provider, err := s.selectProvider(ctx)
	if err != nil {
		return err
	}
	name := newName
	if name == "" {
		name = provider
		if active, ok := original.Providers[original.ActiveProvider]; ok && active.Type == provider {
			name = original.ActiveProvider
		}
	}
	before, exists := original.Providers[name]
	if exists && before.Type != provider {
		return fmt.Errorf("provider %q has a different type", name)
	}
	profile := before
	profile.Type = provider
	// This snapshot is only for prompts and model discovery, never for saving.
	original.ActiveProvider = name
	original.Providers[name] = profile
	if err := s.configureProvider(ctx, original, provider); err != nil {
		return err
	}
	profile = original.Providers[name]
	if err := saveSetupProfile(ctx, setupProfileChange{name: name, before: before, after: profile, existed: exists, activate: newName == ""}); err != nil {
		return err
	}
	return printSetupSaved()
}

type setupProfileChange struct {
	name              string
	before, after     config.ProviderConfig
	existed, activate bool
}

func saveSetupProfile(ctx context.Context, change setupProfileChange) error {
	name := change.name
	if err := config.UpdateContext(ctx, func(latest *config.Config) error {
		current, present := latest.Providers[name]
		switch {
		case !change.existed && present:
			return fmt.Errorf(locale.Text(locale.KeyProviderAlreadyExists), name)
		case change.existed && !present:
			return fmt.Errorf(locale.Text(locale.KeyProviderMissing), name)
		case change.existed && current.Type != change.after.Type:
			return fmt.Errorf("provider %q changed type during setup", name)
		case change.existed:
			latest.Providers[name] = mergeProviderChanges(current, change.before, change.after)
		default:
			latest.Providers[name] = change.after
		}
		if change.activate {
			latest.ActiveProvider = name
		}
		return nil
	}); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func (s *Service) selectProvider(ctx context.Context) (string, error) {
	providers := []string{providerOpenAI, providerGemini, providerAnthropic, providerOllama, providerOpenCode}
	items := make([]list.Item, len(providers))
	for i, provider := range providers {
		items[i] = list.NewSimpleItem(provider, "")
	}
	selected, err := s.selectContext(ctx, list.SelectOptions{Title: locale.Text(locale.KeyUISetupProvider), Items: items})
	if err != nil {
		return "", fmt.Errorf("failed to select provider: %w", err)
	}
	item, ok := selected.(list.SimpleItem)
	if !ok {
		return "", fmt.Errorf("unexpected provider selection type %T", selected)
	}
	return item.Title(), nil
}

func (s *Service) configureProvider(ctx context.Context, cfg *config.Config, kind string) error {
	switch kind {
	case providerOpenAI:
		return s.setupOpenAI(ctx, cfg)
	case providerGemini:
		return s.setupGemini(ctx, cfg)
	case providerAnthropic:
		return s.setupAnthropic(ctx, cfg)
	case providerOllama:
		defaults := config.DefaultConfig()
		return s.promptProvider(ctx, cfg, providerPrompts{urlPrompt: locale.KeyUISetupOllamaBaseURL, defaultURL: defaults.OllamaBaseURL})
	case providerOpenCode:
		return s.setupOpenCode(ctx, cfg)
	default:
		return fmt.Errorf("unsupported provider %q", kind)
	}
}

type providerPrompts struct {
	label, defaultURL, defaultModel string
	urlPrompt                       locale.Key
}

func (s *Service) setupOpenAI(ctx context.Context, cfg *config.Config) error {
	defaults := config.DefaultConfig()
	return s.promptProvider(ctx, cfg, providerPrompts{label: "OpenAI", defaultURL: defaults.OpenAIBaseURL, defaultModel: defaults.OpenAIModel, urlPrompt: locale.KeyUISetupBaseURL})
}

func (s *Service) setupGemini(ctx context.Context, cfg *config.Config) error {
	defaults := config.DefaultConfig()
	return s.promptProvider(ctx, cfg, providerPrompts{label: "Gemini", defaultURL: defaults.GeminiBaseURL, defaultModel: defaults.GeminiModel, urlPrompt: locale.KeyUISetupBaseURL})
}

func (s *Service) setupAnthropic(ctx context.Context, cfg *config.Config) error {
	defaults := config.DefaultConfig()
	return s.promptProvider(ctx, cfg, providerPrompts{label: "Anthropic", defaultURL: defaults.AnthropicBaseURL, defaultModel: defaults.AnthropicModel, urlPrompt: locale.KeyUISetupBaseURL})
}

func (s *Service) setupOpenCode(ctx context.Context, cfg *config.Config) error {
	profile := cfg.Providers[cfg.ActiveProvider]
	profile.BaseURL = "https://opencode.ai/inference/openai/v1"
	profile.HTTPHeaders = map[string]string{
		"User-Agent":         "opencode/latest/2.0.29/cli",
		"x-opencode-session": "ses_ebeea89daffe69kTavLoPHLIzF",
	}
	cfg.Providers[cfg.ActiveProvider] = profile
	return s.promptProvider(ctx, cfg, providerPrompts{label: "OpenCode", defaultModel: "big-pickle"})
}

func (s *Service) promptProvider(ctx context.Context, cfg *config.Config, prompts providerPrompts) error {
	profile := cfg.Providers[cfg.ActiveProvider]
	if prompts.label != "" {
		key, err := s.promptAPIKey(ctx, prompts.label, profile.APIKey)
		if err != nil {
			return err
		}
		profile.APIKey = key
	}
	if prompts.urlPrompt != "" {
		url, err := s.promptWithDefault(ctx, prompts.urlPrompt, profile.BaseURL, prompts.defaultURL)
		if err != nil {
			return err
		}
		profile.BaseURL = url
	}
	model, err := s.promptModel(ctx, profile.Type, profile.APIKey, profile.BaseURL, cfg.EffectiveHTTPHeaders(), profile.Model, prompts.defaultModel)
	if err != nil {
		return err
	}
	profile.Model = model
	cfg.Providers[cfg.ActiveProvider] = profile
	return nil
}

func printSetupSaved() error {
	if _, err := fmt.Fprintln(os.Stdout, locale.Text(locale.KeyUISetupSaved)); err != nil {
		return fmt.Errorf("failed to print success message: %w", err)
	}
	return nil
}
