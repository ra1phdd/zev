package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"gopkg.in/yaml.v3"
)

const (
	existingProfile       = "existing-provider"
	setupOpenAIModel      = "openai-fixture-model"
	setupOpenAIBaseURL    = "https://openai.fixture/v1"
	setupOpenAIDefaultURL = "https://api.openai.com/v1"
	setupGeminiModel      = "gemini-fixture-model"
	setupGeminiBaseURL    = "https://gemini.fixture/v1"
	setupAnthropicModel   = "anthropic-fixture-model"
	setupAnthropicBaseURL = "https://anthropic.fixture/v1"
	setupOllamaBaseURL    = "http://ollama.fixture:11434"
	setupOllamaModel      = "llama3.2-fixture"
	setupOpenCodeModel    = "opencode-fixture-model"
	setupExecutionMode    = "execute"
)

//nolint:paralleltest // Uses process-wide HOME and stdout.
func TestRunContextPersistsEveryProviderConfiguration(t *testing.T) {
	cases := []struct {
		name          string
		provider      string
		inputs        []string
		secret        string
		wantSecret    bool
		wantAPIKey    string
		wantModel     string
		wantBaseURL   string
		wantHeaders   map[string]string
		wantInputText []string
	}{
		{
			name:          "OpenAI",
			provider:      providerOpenAI,
			inputs:        []string{setupOpenAIBaseURL, setupOpenAIModel},
			secret:        "openai-fixture-secret",
			wantSecret:    true,
			wantAPIKey:    "openai-fixture-secret",
			wantModel:     setupOpenAIModel,
			wantBaseURL:   setupOpenAIBaseURL,
			wantInputText: []string{setupOpenAIBaseURL, setupOpenAIModel},
		},
		{
			name:          "OpenAI keeps secret and restores defaults",
			provider:      providerOpenAI,
			inputs:        []string{"default", ""},
			secret:        "",
			wantSecret:    true,
			wantAPIKey:    "saved-openai-secret",
			wantModel:     "saved-openai-model",
			wantBaseURL:   setupOpenAIDefaultURL,
			wantInputText: []string{"default", ""},
		},
		{
			name:          "Gemini",
			provider:      providerGemini,
			inputs:        []string{setupGeminiBaseURL, setupGeminiModel},
			secret:        "gemini-fixture-secret",
			wantSecret:    true,
			wantAPIKey:    "gemini-fixture-secret",
			wantModel:     setupGeminiModel,
			wantBaseURL:   setupGeminiBaseURL,
			wantInputText: []string{setupGeminiBaseURL, setupGeminiModel},
		},
		{
			name:          "Anthropic",
			provider:      providerAnthropic,
			inputs:        []string{setupAnthropicBaseURL, setupAnthropicModel},
			secret:        "anthropic-fixture-secret",
			wantSecret:    true,
			wantAPIKey:    "anthropic-fixture-secret",
			wantModel:     setupAnthropicModel,
			wantBaseURL:   setupAnthropicBaseURL,
			wantInputText: []string{setupAnthropicBaseURL, setupAnthropicModel},
		},
		{
			name:          "Ollama",
			provider:      providerOllama,
			inputs:        []string{setupOllamaBaseURL, setupOllamaModel},
			wantModel:     setupOllamaModel,
			wantBaseURL:   setupOllamaBaseURL,
			wantInputText: []string{setupOllamaBaseURL, setupOllamaModel},
		},
		{
			name:        "OpenCode",
			provider:    providerOpenCode,
			inputs:      []string{setupOpenCodeModel},
			secret:      "opencode-fixture-secret",
			wantSecret:  true,
			wantAPIKey:  "opencode-fixture-secret",
			wantModel:   setupOpenCodeModel,
			wantBaseURL: "https://opencode.ai/inference/openai/v1",
			wantHeaders: map[string]string{
				"User-Agent":         "opencode/latest/2.0.29/cli",
				"x-opencode-session": "ses_ebeea89daffe69kTavLoPHLIzF",
			},
			wantInputText: []string{setupOpenCodeModel},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			original := setupPersistedConfig(t)
			var prompts []string
			fixture := setupInputFixture{values: append([]string(nil), tt.inputs...)}
			secretOutput := new(bytes.Buffer)
			secretCalled := false

			svc := &Service{
				inputContext: fixture.input,
				selectContext: func(ctx context.Context, opts list.SelectOptions) (list.Item, error) {
					return selectProviderFixture(ctx, opts, tt.provider, &prompts)
				},
			}
			if tt.wantSecret {
				svc.runSecretInput = func(model tea.Model) (tea.Model, error) {
					secretCalled = true
					return runSecretInputFixture(model, tt.secret, secretOutput)
				}
			}

			stdout, err := runWithCapturedStdout(t, svc.RunContext)
			if err != nil {
				t.Fatalf("RunContext failed: %v", err)
			}
			if !strings.Contains(stdout, locale.Text(locale.KeyUISetupSaved)) {
				t.Fatalf("success output = %q, want saved message", stdout)
			}
			if got, want := fixture.inputs, tt.wantInputText; !reflect.DeepEqual(got, want) {
				t.Fatalf("input fixture values = %#v, want %#v", got, want)
			}
			if len(prompts) != 1 || len(prompts[0]) == 0 {
				t.Fatalf("provider selection fixture was not called with options: %#v", prompts)
			}
			if secretCalled != tt.wantSecret {
				t.Fatalf("secret input called = %v, want %v", secretCalled, tt.wantSecret)
			}
			if tt.secret != "" && strings.Contains(secretOutput.String(), tt.secret) {
				t.Fatal("secret input output disclosed the entered secret")
			}
			if tt.provider == providerOllama && promptsContain(fixture.prompts, "api key") {
				t.Fatal("Ollama setup prompted for an API key")
			}

			got, err := config.LoadContext(t.Context())
			if err != nil {
				t.Fatalf("Load after RunContext failed: %v", err)
			}
			want := *original
			profileName := tt.provider
			if original.Providers[original.ActiveProvider].Type == tt.provider {
				profileName = original.ActiveProvider
			}
			want.ActiveProvider = profileName
			want.Providers = maps.Clone(original.Providers)
			want.Providers[profileName] = config.ProviderConfig{
				Type: tt.provider, APIKey: tt.wantAPIKey, Model: tt.wantModel,
				BaseURL: tt.wantBaseURL, HTTPHeaders: tt.wantHeaders,
			}
			gotYAML, err := yaml.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			wantYAML, err := yaml.Marshal(&want)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotYAML, wantYAML) {
				t.Fatal("persisted configuration differs from expected public YAML fields")
			}
		})
	}
}

//nolint:paralleltest // Uses process-wide HOME and stdout.
func TestRunContextPreservesPersistedConfigWhenInputFails(t *testing.T) {
	original := setupPersistedConfig(t)
	inputErr := errors.New("fixture input failed")
	fixture := setupInputFixture{err: inputErr}
	svc := &Service{
		inputContext: fixture.input,
		selectContext: func(ctx context.Context, opts list.SelectOptions) (list.Item, error) {
			return selectProviderFixture(ctx, opts, providerOllama, nil)
		},
	}

	stdout, err := runWithCapturedStdout(t, svc.RunContext)
	if err == nil || !errors.Is(err, inputErr) {
		t.Fatalf("RunContext error = %v, want fixture input error", err)
	}
	if stdout != "" {
		t.Fatalf("unexpected output after failed setup: %q", stdout)
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatalf("Load after failed setup failed: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("config changed after failed setup: got %#v, want %#v", got, original)
	}
}

//nolint:paralleltest // Uses process-wide HOME and stdout.
func TestRunContextReportsSelectionAndLoadErrors(t *testing.T) {
	t.Run("selection error", func(t *testing.T) {
		setupPersistedConfig(t)
		selectionErr := errors.New("fixture selection failed")
		svc := &Service{
			selectContext: func(context.Context, list.SelectOptions) (list.Item, error) {
				return nil, selectionErr
			},
		}

		stdout, err := runWithCapturedStdout(t, svc.RunContext)
		if err == nil || !errors.Is(err, selectionErr) {
			t.Fatalf("RunContext error = %v, want selection error", err)
		}
		if stdout != "" {
			t.Fatalf("unexpected output after selection error: %q", stdout)
		}
	})

	t.Run("invalid persisted config", func(t *testing.T) {
		setupPersistedConfig(t)
		path := filepath.Join(os.Getenv("HOME"), ".zev", "config.yaml")
		if err := os.WriteFile(path, []byte("providers: [invalid\n"), 0o600); err != nil { //nolint:gosec // The test controls HOME and the resulting path.
			t.Fatal(err)
		}
		svc := New()

		stdout, err := runWithCapturedStdout(t, svc.RunContext)
		if err == nil || !strings.Contains(err.Error(), "load config") {
			t.Fatalf("RunContext error = %v, want load config error", err)
		}
		if stdout != "" {
			t.Fatalf("unexpected output after load error: %q", stdout)
		}
	})
}

//nolint:paralleltest // Uses process-wide HOME and stdout.
func TestRunContextRejectsUnexpectedProviderSelection(t *testing.T) {
	setupPersistedConfig(t)
	svc := &Service{
		selectContext: func(context.Context, list.SelectOptions) (list.Item, error) {
			return invalidSetupItem{}, nil
		},
	}

	_, err := runWithCapturedStdout(t, svc.RunContext)
	if err == nil || !strings.Contains(err.Error(), "unexpected provider selection type") {
		t.Fatalf("RunContext error = %v, want unexpected selection type", err)
	}
}

//nolint:paralleltest // Uses process-wide HOME and stdout.
func TestRunContextSecretCancellationPreservesConfig(t *testing.T) {
	original := setupPersistedConfig(t)
	secretOutput := new(bytes.Buffer)
	svc := &Service{
		selectContext: func(ctx context.Context, opts list.SelectOptions) (list.Item, error) {
			return selectProviderFixture(ctx, opts, providerOpenAI, nil)
		},
		runSecretInput: func(model tea.Model) (tea.Model, error) {
			return runSecretInputFixture(model, "\x03", secretOutput)
		},
	}

	stdout, err := runWithCapturedStdout(t, svc.RunContext)
	if err == nil || !strings.Contains(err.Error(), "read API key: cancelled") {
		t.Fatalf("RunContext error = %v, want API key cancellation", err)
	}
	if stdout != "" {
		t.Fatalf("unexpected output after canceled setup: %q", stdout)
	}
	if strings.Contains(secretOutput.String(), "saved-openai-secret") {
		t.Fatal("existing secret appeared in cancellation output")
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatalf("Load after canceled setup failed: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("config changed after canceled setup: got %#v, want %#v", got, original)
	}
}

func TestRunContextRejectsCancellationBeforeConfigLoad(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := New().RunContext(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("RunContext error = %v, want context cancellation", err)
	}
}

//nolint:paralleltest // Uses process-wide HOME.
func TestRunContextMergesConcurrentProfileChanges(t *testing.T) {
	original := setupPersistedConfig(t)
	const endpoint = "https://concurrent.example/v1"
	headers := map[string]string{"X-Profile": "concurrent"}
	global := map[string]string{"X-Global": "concurrent"}
	fixture := setupInputFixture{values: []string{"", setupOpenAIModel}}
	svc := &Service{
		inputContext: fixture.input,
		selectContext: func(ctx context.Context, opts list.SelectOptions) (list.Item, error) {
			if err := config.UpdateContext(ctx, func(latest *config.Config) error {
				p := latest.Providers[original.ActiveProvider]
				p.APIKey, p.BaseURL, p.HTTPHeaders = "rotated-key", endpoint, headers
				latest.Providers[original.ActiveProvider] = p
				latest.HTTPHeaders = global
				latest.ActiveProvider = providerGemini
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			return selectProviderFixture(ctx, opts, providerOpenAI, nil)
		},
		runSecretInput: func(model tea.Model) (tea.Model, error) {
			return model, nil
		},
	}
	if err := svc.RunContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	p := got.Providers[original.ActiveProvider]
	if p.Model != setupOpenAIModel || p.APIKey != "rotated-key" || p.BaseURL != endpoint || !maps.Equal(p.HTTPHeaders, headers) {
		t.Fatal("expected only the selected model to change on the updated profile")
	}
	if got.ActiveProvider != original.ActiveProvider || !maps.Equal(got.HTTPHeaders, global) || !reflect.DeepEqual(got.Providers[providerGemini], original.Providers[providerGemini]) {
		t.Fatal("selection, global headers or unrelated profile differ from expected state")
	}
}

//nolint:paralleltest // Uses process-wide HOME.
func TestRunContextRejectsProfileConflictsWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prepare   bool
		typeAfter string
	}{
		{name: "removed while prompting"},
		{name: "retyped while prompting", typeAfter: providerGemini},
		{name: "type-named profile already has another type", prepare: true, typeAfter: providerGemini},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := setupPersistedConfig(t)
			update := func(ctx context.Context) {
				if err := config.UpdateContext(ctx, func(latest *config.Config) error {
					if tc.typeAfter == "" {
						delete(latest.Providers, providerOllama)
					} else {
						latest.Providers[providerOllama] = config.ProviderConfig{Type: tc.typeAfter, Model: "replacement"}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.prepare {
				update(t.Context())
			}
			fixture := setupInputFixture{values: []string{setupOllamaBaseURL, setupOllamaModel}}
			svc := &Service{
				inputContext: fixture.input,
				selectContext: func(ctx context.Context, opts list.SelectOptions) (list.Item, error) {
					if !tc.prepare {
						update(ctx)
					}
					return selectProviderFixture(ctx, opts, providerOllama, nil)
				},
			}
			if err := svc.RunContext(t.Context()); err == nil {
				t.Fatal("expected profile conflict")
			}
			got, err := config.LoadContext(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			p, present := got.Providers[providerOllama]
			if got.ActiveProvider != cfg.ActiveProvider || present != (tc.typeAfter != "") || present && (p.Type != tc.typeAfter || p.Model != "replacement") {
				t.Fatal("setup changed a conflicting profile or switched the active selection")
			}
			if tc.prepare && len(fixture.inputs) != 0 {
				t.Fatal("prompted for settings despite known name conflict")
			}
		})
	}
}

type setupInputFixture struct {
	values  []string
	inputs  []string
	prompts []string
	err     error
}

func (f *setupInputFixture) input(ctx context.Context, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("fixture input context: %w", err)
	}
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return "", f.err
	}
	if len(f.values) == 0 {
		return "", errors.New("fixture input exhausted")
	}
	value := f.values[0]
	f.values = f.values[1:]
	f.inputs = append(f.inputs, value)
	return value, nil
}

func selectProviderFixture(ctx context.Context, opts list.SelectOptions, provider string, prompts *[]string) (list.Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fixture selection context: %w", err)
	}
	if prompts != nil {
		*prompts = append(*prompts, opts.Title)
	}
	if len(opts.Items) != 5 {
		return nil, errors.New("provider selection fixture received unexpected item count")
	}
	wantProviders := []string{providerOpenAI, providerGemini, providerAnthropic, providerOllama, providerOpenCode}
	for i, item := range opts.Items {
		if item.Title() != wantProviders[i] {
			return nil, errors.New("provider selection fixture received unexpected providers")
		}
		if item.Title() == provider {
			return item, nil
		}
	}
	return nil, errors.New("provider selection fixture could not find provider")
}

func runSecretInputFixture(model tea.Model, input string, output *bytes.Buffer) (tea.Model, error) {
	result, err := tea.NewProgram(
		model,
		tea.WithInput(strings.NewReader(input+"\r")),
		tea.WithOutput(output),
		tea.WithoutSignalHandler(),
	).Run()
	if err != nil {
		return result, fmt.Errorf("fixture secret input: %w", err)
	}
	return result, nil
}

func promptsContain(prompts []string, want string) bool {
	for _, prompt := range prompts {
		if strings.Contains(strings.ToLower(prompt), want) {
			return true
		}
	}
	return false
}

func setupPersistedConfig(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.Mkdir(filepath.Join(home, ".zev"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.ActiveProvider = existingProfile
	cfg.Providers = map[string]config.ProviderConfig{
		existingProfile:   {Type: providerOpenAI, APIKey: "saved-openai-secret", Model: "saved-openai-model", BaseURL: "https://saved.openai/v1"},
		providerGemini:    {Type: providerGemini, APIKey: "saved-gemini-secret", Model: "saved-gemini-model", BaseURL: "https://saved.gemini/v1"},
		providerAnthropic: {Type: providerAnthropic, APIKey: "saved-anthropic-secret", Model: "saved-anthropic-model", BaseURL: "https://saved.anthropic/v1"},
		providerOllama:    {Type: providerOllama, Model: "saved-ollama-model", BaseURL: "http://saved.ollama:11434"},
	}
	cfg.HTTPHeaders = map[string]string{"X-Saved": testHeaderValue}
	cfg.CommandExecutionMode = setupExecutionMode
	cfg.ScriptEditor = "vim"
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save fixture config failed: %v", err)
	}
	loaded, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatalf("Load fixture config failed: %v", err)
	}
	return loaded
}

func runWithCapturedStdout(t *testing.T, run func(context.Context) error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
		if err := reader.Close(); err != nil {
			t.Errorf("close captured stdout reader: %v", err)
		}
	}()

	ctx := t.Context()
	runErr := run(ctx)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output), runErr
}

type invalidSetupItem struct{}

func (invalidSetupItem) Title() string       { return providerOpenAI }
func (invalidSetupItem) Description() string { return "" }
func (invalidSetupItem) IsWarning() bool     { return false }
