package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxOptionsCommands is the maximum number of alternatives in a response.
	MaxOptionsCommands = 3
	// MaxCommandLength is the maximum UTF-8 byte length of a command.
	MaxCommandLength = 4096
	// MaxExplanationLength is the maximum UTF-8 byte length of an explanation.
	MaxExplanationLength = 2048
	// MaxOptionsResponseBytes is the maximum encoded response size.
	MaxOptionsResponseBytes = 32 * 1024
	queryRefineMarker       = "::REFINE_QUERY::"
)

// Command is a terminal command suggestion with an explanation and safety metadata.
type Command struct {
	Command                  string `json:"command"`
	ShortExplanation         string `json:"short_explanation"`               //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
	IsDangerous              bool   `json:"is_dangerous"`                    //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
	DangerousExplanation     string `json:"dangerous_explanation,omitempty"` //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
	LocallyAssessedDangerous bool   `json:"-" yaml:"-"`
}

// OptionsResponse contains command suggestions or an explanation of an invalid request.
type OptionsResponse struct {
	Commands              []Command `json:"commands"`
	IsValid               bool      `json:"is_valid"`                           //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
	ExplanationIfNotValid string    `json:"explanation_if_not_valid,omitempty"` //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
}

// Validate checks the response invariants required before command selection or execution.
func (r *OptionsResponse) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: response is nil", ErrInvalidOptionsResponse)
	}

	if err := validateResponseString("explanation_if_not_valid", r.ExplanationIfNotValid, MaxExplanationLength); err != nil {
		return err
	}

	if !r.IsValid {
		if err := r.validateInvalidRequest(); err != nil {
			return err
		}
	} else if len(r.Commands) == 0 {
		return fmt.Errorf("%w: valid response requires commands", ErrInvalidOptionsResponse)
	}
	if len(r.Commands) > MaxOptionsCommands {
		return fmt.Errorf("%w: too many commands", ErrInvalidOptionsResponse)
	}

	for _, command := range r.Commands {
		if command.Command == "" || strings.TrimSpace(command.Command) == "" {
			return fmt.Errorf("%w: command must not be empty", ErrInvalidOptionsResponse)
		}
		if err := validateResponseString("command", command.Command, MaxCommandLength); err != nil {
			return err
		}
		if err := validateResponseString("short_explanation", command.ShortExplanation, MaxExplanationLength); err != nil {
			return err
		}
		if err := validateResponseString("dangerous_explanation", command.DangerousExplanation, MaxExplanationLength); err != nil {
			return err
		}
	}

	return validateEncodedResponseSize(r)
}

func (r *OptionsResponse) validateInvalidRequest() error {
	if strings.TrimSpace(r.ExplanationIfNotValid) == "" {
		return fmt.Errorf("%w: invalid response requires an explanation", ErrInvalidOptionsResponse)
	}
	// Preserve the non-executable marker used by the embedded provider prompt.
	if len(r.Commands) > 0 && (len(r.Commands) != 1 || r.Commands[0].Command != queryRefineMarker) {
		return fmt.Errorf("%w: invalid response may only contain the refinement marker", ErrInvalidOptionsResponse)
	}
	return nil
}

func validateEncodedResponseSize(r *OptionsResponse) error {
	encoded, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("%w: encode response: %w", ErrInvalidOptionsResponse, err)
	}
	if len(encoded) > MaxOptionsResponseBytes {
		return fmt.Errorf("%w: response is too large", ErrInvalidOptionsResponse)
	}
	return nil
}

func validateResponseString(field, value string, maxLength int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidOptionsResponse, field)
	}
	if len(value) > maxLength {
		return fmt.Errorf("%w: %s is too long", ErrInvalidOptionsResponse, field)
	}
	for _, character := range value {
		if unicode.IsControl(character) && !isAllowedWhitespace(character) {
			return fmt.Errorf("%w: %s contains a control character", ErrInvalidOptionsResponse, field)
		}
	}
	return nil
}

// isAllowedWhitespace permits line-oriented text while rejecting other control characters.
func isAllowedWhitespace(character rune) bool {
	switch character {
	case '	', '\n', '\r':
		return true
	default:
		return false
	}
}

// CompactJSON returns a minimal conversation summary containing command strings
// or an invalid-request flag.
func (r *OptionsResponse) CompactJSON() string {
	if !r.IsValid {
		encoded, err := json.Marshal(struct {
			IsValid bool `json:"is_valid"` //nolint:tagliatelle // Preserve the provider wire contract.
		}{IsValid: false})
		if err != nil {
			return ""
		}
		return string(encoded)
	}

	commands := make([]string, 0, len(r.Commands))
	for _, cmd := range r.Commands {
		commands = append(commands, cmd.Command)
	}

	encoded, err := json.Marshal(struct {
		Commands []string `json:"commands"`
	}{Commands: commands})
	if err != nil {
		return ""
	}
	return string(encoded)
}
