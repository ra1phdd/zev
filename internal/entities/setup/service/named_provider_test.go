package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/pkg/tui/list"
)

//nolint:paralleltest // Uses process-wide HOME and stdout through setupPersistedConfig.
func TestRunNamedContextAddsProfileWithoutOverwritingExistingProfile(t *testing.T) {
	original := setupPersistedConfig(t)

	fixture := &setupInputFixture{values: []string{"https://named.example/v1", "named-model"}}
	svc := &Service{
		inputContext: fixture.input,
		selectContext: func(ctx context.Context, opts list.SelectOptions) (list.Item, error) {
			return selectProviderFixture(ctx, opts, providerOpenAI, nil)
		},
		runSecretInput: func(model tea.Model) (tea.Model, error) {
			m, ok := model.(apiKeyInput)
			if !ok {
				return nil, errors.New("unexpected input model")
			}
			m.textInput.SetValue("named-secret")
			return m, nil
		},
	}

	if err := svc.RunNamedContext(t.Context(), "work"); err != nil {
		t.Fatalf("RunNamedContext() error = %v", err)
	}
	got, err := config.LoadContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveProvider != original.ActiveProvider {
		t.Fatalf("active provider changed = %q, want %q", got.ActiveProvider, original.ActiveProvider)
	}
	if !reflect.DeepEqual(got.Providers[original.ActiveProvider], original.Providers[original.ActiveProvider]) {
		t.Fatal("existing provider profile was overwritten")
	}
	profile, ok := got.Providers["work"]
	if !ok {
		t.Fatal("named provider profile was not added")
	}
	if profile.Type != providerOpenAI || profile.APIKey != "named-secret" || profile.Model != "named-model" || profile.BaseURL != "https://named.example/v1" {
		t.Fatalf("named profile = %#v", profile)
	}
}

//nolint:paralleltest // Uses process-wide HOME and stdout through setupPersistedConfig.
func TestRunNamedContextRejectsExistingNameBeforePrompting(t *testing.T) {
	setupPersistedConfig(t)
	called := false
	svc := &Service{
		selectContext: func(context.Context, list.SelectOptions) (list.Item, error) {
			called = true
			return nil, errors.New("selection must not run")
		},
	}

	if err := svc.RunNamedContext(t.Context(), existingProfile); err == nil {
		t.Fatal("RunNamedContext() accepted an existing provider name")
	}
	if called {
		t.Fatal("provider selection ran before duplicate-name rejection")
	}
}
