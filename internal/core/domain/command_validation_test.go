package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestOptionsResponseValidationStringFields(t *testing.T) {
	t.Parallel()

	fields := []struct {
		name string
		max  int
		set  func(*OptionsResponse, string)
	}{
		{name: "command", max: MaxCommandLength, set: func(r *OptionsResponse, value string) { r.Commands[0].Command = value }},
		{name: "short explanation", max: MaxExplanationLength, set: func(r *OptionsResponse, value string) { r.Commands[0].ShortExplanation = value }},
		{name: "danger explanation", max: MaxExplanationLength, set: func(r *OptionsResponse, value string) { r.Commands[0].DangerousExplanation = value }},
		{name: "invalid explanation", max: MaxExplanationLength, set: func(r *OptionsResponse, value string) { r.ExplanationIfNotValid = value }},
	}
	for _, field := range fields {
		for _, input := range []struct {
			name    string
			value   string
			wantErr bool
		}{
			{name: "at byte limit", value: strings.Repeat("x", field.max)},
			{name: "over byte limit", value: strings.Repeat("x", field.max+1), wantErr: true},
			{name: "multibyte over byte limit", value: strings.Repeat("я", field.max/2+1), wantErr: true},
			{name: "allowed whitespace", value: "line one\nline two\r\tend"},
			{name: "NUL", value: "text\x00", wantErr: true},
			{name: "ESC", value: "text\x1b[31m", wantErr: true},
			{name: "DEL", value: "text\x7f", wantErr: true},
			{name: "C1 control", value: "text\u009b", wantErr: true},
			{name: "invalid UTF-8", value: string([]byte{'x', 0xff}), wantErr: true},
		} {
			t.Run(field.name+"/"+input.name, func(t *testing.T) {
				t.Parallel()
				response := OptionsResponse{IsValid: true, Commands: []Command{{Command: "echo hello"}}}
				field.set(&response, input.value)
				err := response.Validate()
				if (err != nil) != input.wantErr {
					t.Fatalf("Validate() error = %v, want error %t", err, input.wantErr)
				}
				if err != nil && !errors.Is(err, ErrInvalidOptionsResponse) {
					t.Fatalf("Validate() error = %v, want ErrInvalidOptionsResponse", err)
				}
			})
		}
	}
}

func TestOptionsResponseValidationBoundsEncodedSize(t *testing.T) {
	t.Parallel()

	response := OptionsResponse{IsValid: true, Commands: make([]Command, MaxOptionsCommands)}
	for i := range response.Commands {
		response.Commands[i] = Command{
			Command:              strings.Repeat("x", MaxCommandLength),
			ShortExplanation:     strings.Repeat("x", MaxExplanationLength),
			DangerousExplanation: strings.Repeat("x", MaxExplanationLength),
		}
	}
	response.ExplanationIfNotValid = strings.Repeat("x", MaxExplanationLength)
	if err := response.Validate(); err != nil {
		t.Fatalf("maximum ordinary fields rejected: %v", err)
	}
	for i := range response.Commands {
		response.Commands[i].Command = strings.Repeat("<", MaxCommandLength)
	}
	if err := response.Validate(); !errors.Is(err, ErrInvalidOptionsResponse) {
		t.Fatalf("Validate() error = %v, want oversized encoded response error", err)
	}
}

func TestOptionsResponseCompactJSONInvalidRequest(t *testing.T) {
	t.Parallel()

	response := OptionsResponse{IsValid: false}
	if got := response.CompactJSON(); got != `{"is_valid":false}` {
		t.Fatalf("CompactJSON() = %s, want invalid-request flag", got)
	}
}
