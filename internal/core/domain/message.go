package domain

// Message contains a conversation role and its text.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // Message text.
}

// ConversationHistory contains the system prompt and subsequent conversation turns.
type ConversationHistory struct {
	SystemPrompt string    // Persistent system prompt.
	Messages     []Message // User and assistant messages.
}
