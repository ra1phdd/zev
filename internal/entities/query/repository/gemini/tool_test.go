package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestCompleteToolTurnExchange(t *testing.T) {
	t.Parallel()
	const arguments = `{"command":"pwd","args":[],"reason":"Locate project"}`
	const final = `{"commands":[],"is_valid":false,"explanation_if_not_valid":"Need details"}`
	requests := make(chan map[string]json.RawMessage, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/models/test-model:generateContent" || r.URL.RawQuery != "" {
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("missing native authentication header")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- body
		response := `{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":"Inspecting"},{"functionCall":{"id":"call-1","name":"exec","args":` + arguments + `},"thoughtSignature":"opaque-signature"}]}}]}`
		var contents []json.RawMessage
		if err := json.Unmarshal(body[geminiJSONContents], &contents); err != nil {
			t.Errorf("decode contents: %v", err)
			return
		}
		if len(contents) > 3 {
			response = `{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":` + jsonString(t, final) + `}]}}]}`
		}
		if _, err := io.WriteString(w, response); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := New("test-key", "test-model", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := any(provider).(domain.ToolProvider)
	if !ok {
		t.Fatal("provider does not support native tool turns")
	}
	history := &domain.ConversationHistory{SystemPrompt: "original-system", Messages: []domain.Message{
		{Role: geminiRoleUser, Content: "original-user"}, {Role: geminiRoleAssistant, Content: "original-assistant"}, {Role: geminiRoleUser, Content: "refinement"},
	}}
	before := append([]domain.Message(nil), history.Messages...)
	request := domain.ToolRequest{History: history, EnableTools: true}
	turn, err := tools.CompleteToolTurn(t.Context(), request)
	if err != nil || turn == nil {
		t.Fatalf("first turn = (%v, %v)", turn, err)
	}
	wantCall := domain.ToolCall{ID: "call-1", Name: geminiExecName, Arguments: arguments, Signature: "opaque-signature"}
	if turn.Content != "Inspecting" || !reflect.DeepEqual(turn.Calls, []domain.ToolCall{wantCall}) {
		t.Fatalf("first turn = %+v", turn)
	}
	first := <-requests
	var definitions []struct {
		Functions []struct {
			Name   string         `json:"name"`
			Schema map[string]any `json:"parametersJsonSchema"`
		} `json:"functionDeclarations"`
	}
	if err := json.Unmarshal(first["tools"], &definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || len(definitions[0].Functions) != 1 || definitions[0].Functions[0].Name != geminiExecName ||
		!reflect.DeepEqual(definitions[0].Functions[0].Schema, jsonObject(t, domain.ExecToolSchema())) {
		t.Fatalf("tool definitions = %+v", definitions)
	}
	if _, exists := first["generationConfig"]; exists {
		t.Fatal("tool requests must not force a JSON response schema")
	}
	request.Messages = []domain.ToolMessage{
		{Role: geminiRoleAssistant, Content: turn.Content, ToolCalls: turn.Calls},
		{Role: geminiRoleTool, Content: `{"stdout":"/project"}`, ToolCallID: wantCall.ID},
	}
	turn, err = tools.CompleteToolTurn(t.Context(), request)
	if err != nil || turn == nil || turn.Content != final || len(turn.Calls) != 0 {
		t.Fatalf("final turn = (%+v, %v)", turn, err)
	}
	second := <-requests
	var system struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(second[geminiJSONSystem], &system); err != nil {
		t.Fatal(err)
	}
	if system.Role != "" || len(system.Parts) != 1 || system.Parts[0].Text != history.SystemPrompt {
		t.Fatalf("system instruction = %+v", system)
	}
	var systemFields map[string]json.RawMessage
	if err := json.Unmarshal(second[geminiJSONSystem], &systemFields); err != nil {
		t.Fatal(err)
	}
	if _, exists := systemFields["role"]; exists {
		t.Fatal("system instruction must not send a conversation role")
	}
	var contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text      string `json:"text"`
			Signature string `json:"thoughtSignature"`
			Call      *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"functionCall"`
			Result *struct {
				ID       string         `json:"id"`
				Name     string         `json:"name"`
				Response map[string]any `json:"response"`
			} `json:"functionResponse"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(second[geminiJSONContents], &contents); err != nil {
		t.Fatal(err)
	}
	if len(contents) != 5 || contents[0].Parts[0].Text != "original-user" || contents[1].Role != geminiRoleModel ||
		contents[3].Parts[1].Signature != wantCall.Signature || contents[3].Parts[1].Call == nil || contents[3].Parts[1].Call.ID != wantCall.ID ||
		contents[4].Role != geminiRoleUser || contents[4].Parts[0].Result == nil || contents[4].Parts[0].Result.ID != wantCall.ID ||
		contents[4].Parts[0].Result.Name != geminiExecName || contents[4].Parts[0].Result.Response["stdout"] != "/project" {
		t.Fatalf("native exchange = %+v", contents)
	}
	if !reflect.DeepEqual(history.Messages, before) {
		t.Fatal("original history was mutated")
	}
}

func jsonString(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
