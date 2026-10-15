package domain

// Message представляет одно сообщение в диалоге с LLM.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // текст сообщения
}

// ConversationHistory представляет историю диалога для уточнения запроса.
type ConversationHistory struct {
	SystemPrompt string    // системный промпт (постоянный)
	Messages     []Message // история сообщений (user/assistant)
}
