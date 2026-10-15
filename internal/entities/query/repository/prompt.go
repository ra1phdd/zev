package repository

import _ "embed"

//go:embed system_prompt.txt
var SystemPrompt string

//go:embed user_prompt.txt
var UserPromptTemplate string

//go:embed prompt.txt
var PromptTemplate string // deprecated: используется для обратной совместимости
