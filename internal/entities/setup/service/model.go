package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/ra1phdd/zev/internal/core/httpclient"
	"github.com/ra1phdd/zev/internal/core/locale"
	modelclient "github.com/ra1phdd/zev/internal/entities/setup/client"
	"github.com/ra1phdd/zev/pkg/tui/list"
)

type modelListRequest struct {
	Provider string
	APIKey   string
	BaseURL  string
	Headers  map[string]string
	Proxy    string
}

type modelFetcher func(context.Context, modelListRequest) ([]string, error)

func defaultModelFetcher(ctx context.Context, request modelListRequest) ([]string, error) {
	httpClient, err := httpclient.NewWithProxy(request.Headers, request.Proxy)
	if err != nil {
		return nil, fmt.Errorf("configure model-list proxy: %w", err)
	}
	client := modelclient.New(httpClient)
	models, err := client.ListModels(ctx, modelclient.Request{
		Provider: request.Provider,
		APIKey:   request.APIKey,
		BaseURL:  request.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("list provider models: %w", err)
	}
	return models, nil
}

func (s *Service) promptModel(ctx context.Context, provider, apiKey, baseURL string, headers map[string]string, current, defaultVal string) (string, error) {
	if s.listModels == nil {
		return s.promptWithDefault(ctx, locale.KeyUISetupModel, current, defaultVal)
	}

	models, err := s.listModels(ctx, modelListRequest{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
		Headers:  maps.Clone(headers),
		Proxy:    s.proxyURL,
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("list models: %w", ctxErr)
		}
		if s.warn != nil {
			s.warn(fmt.Sprintf(locale.Text(locale.KeyUISetupModelLoadFailed), err))
		}
		return s.promptWithDefault(ctx, locale.KeyUISetupModel, current, defaultVal)
	}
	if len(models) == 0 {
		return s.promptWithDefault(ctx, locale.KeyUISetupModel, current, defaultVal)
	}

	items := modelSelectionItems(models, current, defaultVal)
	selected, err := s.selectContext(ctx, list.SelectOptions{
		Title: locale.Text(locale.KeyUISetupSelectModel),
		Items: items,
	})
	if err != nil {
		return "", fmt.Errorf("failed to select model: %w", err)
	}
	item, ok := selected.(modelSelectionItem)
	if ok {
		if item.manual {
			return s.promptWithDefault(ctx, locale.KeyUISetupModel, current, defaultVal)
		}
		return item.model, nil
	}
	if selected == nil || strings.TrimSpace(selected.Title()) == "" {
		return "", errors.New("unexpected model selection")
	}
	return selected.Title(), nil
}

type modelSelectionItem struct {
	model  string
	manual bool
}

func (i modelSelectionItem) Title() string {
	if i.manual {
		return locale.Text(locale.KeyUISetupManualModel)
	}
	return i.model
}

func (i modelSelectionItem) Description() string { return "" }
func (i modelSelectionItem) IsWarning() bool     { return false }

func modelSelectionItems(models []string, current, defaultVal string) []list.Item {
	seen := make(map[string]struct{}, len(models)+2)
	ordered := make([]string, 0, len(models)+2)
	appendModel := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, exists := seen[model]; exists {
			return
		}
		seen[model] = struct{}{}
		ordered = append(ordered, model)
	}
	if current != "" {
		appendModel(current)
	} else if defaultVal != "" {
		appendModel(defaultVal)
	}
	for _, model := range models {
		appendModel(model)
	}

	items := make([]list.Item, 0, len(ordered)+1)
	for _, model := range ordered {
		items = append(items, modelSelectionItem{model: model})
	}
	items = append(items, modelSelectionItem{manual: true})
	return items
}
