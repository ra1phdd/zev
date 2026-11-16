// Package domain defines command suggestions, conversation data, and provider contracts.
package domain

// Alias is a named command template with argument substitution.
type Alias struct {
	Name     string
	Template string
}

// Script describes a named Bash script and its backing file.
type Script struct {
	Name        string
	ScriptFile  string
	Description string
}
