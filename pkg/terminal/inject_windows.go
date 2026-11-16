//go:build windows

package terminal

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// Inject validates command text but cannot insert it on Windows.
// It returns ErrInjectionFailed for malformed text or any control character,
// including newlines and tabs, and ErrInjectionUnsupported otherwise.
// Callers can distinguish these errors using errors.Is.
func Inject(text string) error {
	response := domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: text}}}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInjectionFailed, err)
	}
	if strings.ContainsFunc(text, unicode.IsControl) {
		return fmt.Errorf("%w: terminal control characters are not allowed", ErrInjectionFailed)
	}
	return ErrInjectionUnsupported
}
