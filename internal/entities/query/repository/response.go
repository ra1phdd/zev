package repository

import (
	"encoding/json"
	"fmt"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// DecodeOptionsResponse extracts, decodes, and validates one provider response.
// It never includes provider response contents in the returned error.
func DecodeOptionsResponse(providerName, text string) (*domain.OptionsResponse, error) {
	if len(text) > MaxProviderResponseBytes {
		return nil, WrapResponseError(providerName, ErrProviderResponseTooLarge)
	}
	cleanJSON := ExtractJSON(text)
	if len(cleanJSON) > domain.MaxOptionsResponseBytes {
		return nil, WrapResponseError(providerName, fmt.Errorf("%w: response is too large", domain.ErrInvalidOptionsResponse))
	}

	var response domain.OptionsResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, WrapResponseError(providerName, err)
	}
	if err := ValidateOptionsResponse(providerName, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ValidateOptionsResponse validates provider data before it reaches query selection or execution.
// The returned error keeps the validation sentinel available while hiding response contents.
func ValidateOptionsResponse(providerName string, response *domain.OptionsResponse) error {
	if err := response.Validate(); err != nil {
		return WrapResponseError(providerName, err)
	}
	return nil
}
