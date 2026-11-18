package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/ra1phdd/zev/pkg/tui/list"
)

const (
	setupModelDuplicate = "model-b"
	testHeaderName      = "X-Test"
	testHeaderValue     = "header"
)

func TestModelSelectionItemsKeepsCurrentAndAddsManualFallback(t *testing.T) {
	t.Parallel()

	items := modelSelectionItems([]string{setupModelDuplicate, "model-a", setupModelDuplicate}, "model-current", "model-default")
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Title())
	}
	want := []string{"model-current", setupModelDuplicate, "model-a", locale.Text(locale.KeyUISetupManualModel)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model selection items = %#v, want %#v", got, want)
	}
}

func TestPromptModelSelectsLoadedModel(t *testing.T) {
	t.Parallel()

	var requested modelListRequest
	s := &Service{
		listModels: func(_ context.Context, request modelListRequest) ([]string, error) {
			requested = request
			return []string{"server-model"}, nil
		},
		selectContext: func(_ context.Context, opts list.SelectOptions) (list.Item, error) {
			if opts.Title != locale.Text(locale.KeyUISetupSelectModel) {
				t.Fatalf("selection title = %q", opts.Title)
			}
			return opts.Items[1], nil
		},
	}

	got, err := s.promptModel(t.Context(), providerOpenAI, "secret", "https://provider.example/v1", map[string]string{testHeaderName: testHeaderValue}, "saved-model", "default-model")
	if err != nil {
		t.Fatalf("promptModel() error = %v", err)
	}
	if got != "server-model" {
		t.Fatalf("selected model = %q, want server model", got)
	}
	want := modelListRequest{Provider: providerOpenAI, APIKey: "secret", BaseURL: "https://provider.example/v1", Headers: map[string]string{testHeaderName: testHeaderValue}}
	if !reflect.DeepEqual(requested, want) {
		t.Fatalf("model request = %#v, want %#v", requested, want)
	}
}

func TestPromptModelFallsBackToManualInputWhenCatalogFails(t *testing.T) {
	t.Parallel()

	inputCalled := false
	s := &Service{
		listModels: func(context.Context, modelListRequest) ([]string, error) {
			return nil, errors.New("catalog unavailable")
		},
		inputContext: func(_ context.Context, prompt string) (string, error) {
			inputCalled = true
			if prompt == "" {
				t.Fatal("empty manual model prompt")
			}
			return "manual-model", nil
		},
	}

	got, err := s.promptModel(t.Context(), providerOllama, "", "http://localhost:11434", nil, "", "")
	if err != nil {
		t.Fatalf("promptModel() error = %v", err)
	}
	if got != "manual-model" || !inputCalled {
		t.Fatalf("manual fallback = %q, input called = %v", got, inputCalled)
	}
}
