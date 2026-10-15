package domain

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
