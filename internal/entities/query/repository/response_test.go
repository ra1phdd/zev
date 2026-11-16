package repository_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
)

func TestDecodeOptionsResponseUsesSafeValidation(t *testing.T) {
	t.Parallel()

	const secret = "secret-key"
	payload, err := json.Marshal(domain.OptionsResponse{
		Commands: []domain.Command{{Command: secret}, {Command: ""}},
		IsValid:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.DecodeOptionsResponse("test-provider", string(payload))
	if err == nil {
		t.Fatal("DecodeOptionsResponse() error = nil, want invalid response error")
	}
	if !errors.Is(err, domain.ErrInvalidOptionsResponse) {
		t.Fatalf("error = %v, want ErrInvalidOptionsResponse", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed response contents: %v", err)
	}
}

func TestDecodeOptionsResponseRejectsMalformedPayloads(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{
		`null`,
		`{}`,
		`{"commands":[],"is_valid":true}`,
		`{"commands":[null],"is_valid":true}`,
		`{"commands":[{"command":42}],"is_valid":true}`,
		`{"commands":[{"command":"echo ok"}],"is_valid":true} {}`,
		`{"commands":[{"command":"echo ok"}],"is_valid":true`,
		strings.Repeat("x", domain.MaxOptionsResponseBytes+1),
	} {
		t.Run(payload[:min(len(payload), 48)], func(t *testing.T) {
			t.Parallel()
			response, err := repository.DecodeOptionsResponse("test-provider", payload)
			if err == nil || response != nil {
				t.Fatalf("DecodeOptionsResponse() = (%v, %v), want nil response and error", response, err)
			}
		})
	}
}

func TestDecodeOptionsResponsePreservesCommandText(t *testing.T) {
	t.Parallel()

	command := "printf \"quoted\\\\path\"\n"
	payload, err := json.Marshal(domain.OptionsResponse{
		Commands: []domain.Command{{Command: command, ShortExplanation: "Print a quoted path"}},
		IsValid:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := repository.DecodeOptionsResponse("test-provider", string(payload))
	if err != nil {
		t.Fatalf("DecodeOptionsResponse() error = %v", err)
	}
	if response.Commands[0].Command != command {
		t.Fatalf("command = %q, want %q", response.Commands[0].Command, command)
	}
}
