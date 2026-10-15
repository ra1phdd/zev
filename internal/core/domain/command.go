package domain

import "strings"

// Command представляет одну команду терминала с пояснением.
type Command struct {
	Command              string `json:"command"`
	ShortExplanation     string `json:"short_explanation"`
	IsDangerous          bool   `json:"is_dangerous"`
	DangerousExplanation string `json:"dangerous_explanation,omitempty"`
}

// OptionsResponse представляет ответ от LLM провайдера.
type OptionsResponse struct {
	Commands              []Command `json:"commands"`
	IsValid               bool      `json:"is_valid"`
	ExplanationIfNotValid string    `json:"explanation_if_not_valid,omitempty"`
}

// CompactJSON возвращает минималистичное JSON-представление для экономии токенов.
func (r *OptionsResponse) CompactJSON() string {
	if !r.IsValid {
		return `{"is_valid":false}`
	}
	// Сокращаем до списка команд без пояснений
	var commands []string
	for _, cmd := range r.Commands {
		commands = append(commands, cmd.Command)
	}
	// Простая сериализация без encoding/json для экономии байтов
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
