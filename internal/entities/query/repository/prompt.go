package repository

import _ "embed"

// SystemPrompt contains the embedded system instruction template.
// It has one %s placeholder for the environment context.
//
//go:embed system_prompt.txt
var SystemPrompt string

// UserPromptTemplate keeps the user message as the original prompt only.
const UserPromptTemplate = "%s"
