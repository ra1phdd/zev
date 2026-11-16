package domain

import (
	"errors"
	"testing"
)

const (
	invalidExplanation = "Please clarify"
	fullExplanation    = "Please clarify the target"
	echoHello          = "echo hello"
	refineMarker       = queryRefineMarker
)

func TestOptionsResponseInvalidRequestSemantics(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		response OptionsResponse
		wantErr  bool
	}{
		{name: "explanation only", response: OptionsResponse{ExplanationIfNotValid: fullExplanation}},
		{name: "empty commands", response: OptionsResponse{Commands: []Command{}, ExplanationIfNotValid: fullExplanation}},
		{name: "legacy refinement marker", response: OptionsResponse{Commands: []Command{{Command: refineMarker, ShortExplanation: "Which target?"}}, ExplanationIfNotValid: fullExplanation}},
		{name: "missing explanation", response: OptionsResponse{}, wantErr: true},
		{name: "blank explanation", response: OptionsResponse{ExplanationIfNotValid: " \n\t"}, wantErr: true},
		{name: "executable command", response: OptionsResponse{Commands: []Command{{Command: echoHello}}, ExplanationIfNotValid: invalidExplanation}, wantErr: true},
		{name: "mixed commands", response: OptionsResponse{Commands: []Command{{Command: refineMarker}, {Command: echoHello}}, ExplanationIfNotValid: invalidExplanation}, wantErr: true},
		{name: "duplicate markers", response: OptionsResponse{Commands: []Command{{Command: refineMarker}, {Command: refineMarker}}, ExplanationIfNotValid: invalidExplanation}, wantErr: true},
		{name: "empty command", response: OptionsResponse{Commands: []Command{{}}, ExplanationIfNotValid: invalidExplanation}, wantErr: true},
		{name: "valid without commands", response: OptionsResponse{IsValid: true, ExplanationIfNotValid: invalidExplanation}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.response.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, want error %t", err, test.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidOptionsResponse) {
				t.Fatalf("Validate() error = %v, want ErrInvalidOptionsResponse", err)
			}
		})
	}
}
