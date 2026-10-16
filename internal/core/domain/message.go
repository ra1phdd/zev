package domain

type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // текст сообщения
}

type ConversationHistory struct {
	SystemPrompt string    // системный промпт (постоянный)
	Messages     []Message // история сообщений (user/assistant)
}
