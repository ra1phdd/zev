// Package domain defines command suggestions, conversation data, and provider contracts.
package domain

// Argument describes one positional argument accepted by an alias or script.
type Argument struct {
	Name        string
	Description string
	Required    bool
}

// Alias is a named command template with argument substitution.
type Alias struct {
	Name        string
	Template    string
	Description string
	Arguments   []Argument
}

// Script describes a named Bash script and its backing file.
type Script struct {
	Name        string
	RecordID    string
	ScriptFile  string
	Description string
	Arguments   []Argument
}
