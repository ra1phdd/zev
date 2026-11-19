package domain

import "context"

const (
	toolSchemaTypeKey        = "type"
	toolSchemaStringType     = "string"
	toolSchemaDescriptionKey = "description"
	toolSchemaReasonKey      = "reason"
)

// ToolCall is an untrusted provider request to invoke a local tool. Arguments is
// a JSON object and must be validated before execution. Signature is opaque
// provider metadata, not an authorization signal.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
	Signature string
}

// ToolMessage is an ephemeral tool protocol turn. It is not persisted in history.
type ToolMessage struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolRequest contains the original conversation and ephemeral tool exchange.
// EnableTools=false must omit tool definitions and prohibit tool execution; this
// is used for the isolated safety review, whose content is not an options response.
type ToolRequest struct {
	History        *ConversationHistory
	Messages       []ToolMessage
	EnableTools    bool
	EnableWebFetch bool
}

// ToolTurn is a provider response before final command-options decoding.
type ToolTurn struct {
	Content string
	Calls   []ToolCall
}

// ToolProvider optionally supports native tool calls. InferenceProvider remains
// unchanged so clients without tools retain their existing contract.
type ToolProvider interface {
	CompleteToolTurn(ctx context.Context, request ToolRequest) (*ToolTurn, error)
}

// ExecRequest describes one executable and its exact argument vector. Shell
// interpretation happens only when an explicitly approved shell is requested.
type ExecRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Reason  string   `json:"reason"`
}

// ToolResult reports execution or refusal back to the primary model.
type ToolResult struct {
	Output    string                  `json:"output"`
	ExitCode  int                     `json:"exit_code"` //nolint:tagliatelle // Tool wire format uses snake_case.
	Truncated bool                    `json:"truncated"`
	Denied    bool                    `json:"denied"`
	Reason    string                  `json:"reason,omitempty"`
	Security  *ToolSecurityAnnotation `json:"security,omitempty"`
}

// ToolSecurityAnnotation records local inspection of untrusted output.
type ToolSecurityAnnotation struct {
	State    string   `json:"state"`
	Findings []string `json:"findings,omitempty"`
}

// ExecToolSchema returns the native function parameters for the exec tool.
// A fresh map prevents one provider's schema conversion from changing another.
func ExecToolSchema() map[string]any {
	return map[string]any{
		toolSchemaTypeKey: "object",
		"properties": map[string]any{
			"command":           map[string]any{toolSchemaTypeKey: toolSchemaStringType, toolSchemaDescriptionKey: "Executable name, not a shell command line."},
			"args":              map[string]any{toolSchemaTypeKey: "array", "items": map[string]any{toolSchemaTypeKey: toolSchemaStringType}},
			toolSchemaReasonKey: map[string]any{toolSchemaTypeKey: toolSchemaStringType, toolSchemaDescriptionKey: "Why this context is needed."},
		},
		"required":             []string{"command", "args", toolSchemaReasonKey},
		"additionalProperties": false,
	}
}

// WebFetchToolSchema returns the bounded public-web fetch parameters.
func WebFetchToolSchema() map[string]any {
	return map[string]any{
		toolSchemaTypeKey: "object",
		"properties": map[string]any{
			"url":               map[string]any{toolSchemaTypeKey: toolSchemaStringType, toolSchemaDescriptionKey: "Public HTTP(S) URL to fetch with bounded GET."},
			toolSchemaReasonKey: map[string]any{toolSchemaTypeKey: toolSchemaStringType, toolSchemaDescriptionKey: "Why this public page is needed."},
		},
		"required":             []string{"url", toolSchemaReasonKey},
		"additionalProperties": false,
	}
}
