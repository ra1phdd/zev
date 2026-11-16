package domain

import "strings"

// Command is a terminal command suggestion with an explanation and safety metadata.
type Command struct {
	Command              string `json:"command"`
	ShortExplanation     string `json:"short_explanation"`               //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
	IsDangerous          bool   `json:"is_dangerous"`                    //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
	DangerousExplanation string `json:"dangerous_explanation,omitempty"` //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
}

// OptionsResponse contains command suggestions or an explanation of an invalid request.
type OptionsResponse struct {
	Commands              []Command `json:"commands"`
	IsValid               bool      `json:"is_valid"`                           //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
	ExplanationIfNotValid string    `json:"explanation_if_not_valid,omitempty"` //nolint:tagliatelle // Snake_case is the established LLM response and persisted history format.
}

// CompactJSON returns a minimal conversation summary containing command strings
// or an invalid-request flag. Command strings must already be JSON-safe.
func (r *OptionsResponse) CompactJSON() string {
	if !r.IsValid {
		return `{"is_valid":false}`
	}

	var commands []string
	for _, cmd := range r.Commands {
		commands = append(commands, cmd.Command)
	}

	result := `{"commands":[`
	var resultSb30 strings.Builder
	for i, cmd := range commands {
		if i > 0 {
			resultSb30.WriteString(",")
		}
		resultSb30.WriteString(`"` + cmd + `"`)
	}
	result += resultSb30.String()
	result += `]}`
	return result
}
