package service

import (
	"maps"

	"github.com/ra1phdd/zev/internal/core/config"
)

// mergeProviderChanges applies only changed fields from the setup snapshot.
func mergeProviderChanges(latest, original, changed config.ProviderConfig) config.ProviderConfig {
	if original.APIKey != changed.APIKey {
		latest.APIKey = changed.APIKey
	}
	if original.Model != changed.Model {
		latest.Model = changed.Model
	}
	if original.BaseURL != changed.BaseURL {
		latest.BaseURL = changed.BaseURL
	}
	if !maps.Equal(original.HTTPHeaders, changed.HTTPHeaders) {
		latest.HTTPHeaders = maps.Clone(changed.HTTPHeaders)
	}
	return latest
}
