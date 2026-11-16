package service

import (
	"errors"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestServiceRejectsInvalidProviderResponse(t *testing.T) {
	t.Parallel()
	for _, history := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "history"}[history], func(t *testing.T) {
			t.Parallel()
			for _, response := range []*domain.OptionsResponse{nil, {IsValid: true}, {IsValid: true, Commands: []domain.Command{{Command: " \t "}}}, {IsValid: true, Commands: []domain.Command{{Command: "echo \x1b[31m"}}}} {
				svc := New(&mockProvider{response: response})
				var got *domain.OptionsResponse
				var err error
				if history {
					got, err = svc.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{})
				} else {
					got, err = svc.GetOptions(t.Context(), "test")
				}
				if got != nil || !errors.Is(err, domain.ErrInvalidOptionsResponse) {
					t.Fatalf("response=%v error=%v; invalid provider data must be rejected", got, err)
				}
			}
		})
	}
}
