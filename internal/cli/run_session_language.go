package cli

import (
	"context"
	"strings"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
)

func setRunSessionLanguage(ctx context.Context, sessionID string) func() {
	if sessionID == "" {
		return func() {}
	}
	cfg, err := config.LoadContext(ctx)
	if err != nil {
		return func() {}
	}
	conversationID, _, err := resolveRunSession(ctx, sessionID)
	if err != nil {
		return func() {}
	}
	store, err := openQueryHistory(ctx)
	if err != nil {
		return func() {}
	}
	view, err := store.GetConversation(ctx, conversationID)
	if err != nil {
		if closeErr := store.Close(); closeErr != nil {
			return func() {}
		}
		return func() {}
	}
	if err := store.Close(); err != nil {
		return func() {}
	}
	request := ""
	for _, message := range view.Messages {
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			request = message.Content
			break
		}
	}
	restore := locale.SetLanguage(cfg.UILanguage, request)
	return restore
}
