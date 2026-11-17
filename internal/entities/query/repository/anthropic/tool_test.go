package anthropic

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
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("missing native authentication/version headers")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- body
		response := `{"type":"message","role":"assistant","stop_reason":"tool_use","content":[{"type":"text","text":"Inspecting"},{"type":"tool_use","id":"call-1","name":"exec","input":` + arguments + `}]}`
		var messages []json.RawMessage
		if err := json.Unmarshal(body["messages"], &messages); err != nil {
			t.Errorf("decode messages: %v", err)
			return
		}
		if len(messages) > 3 {
			response = `{"type":"message","role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":` + jsonString(t, final) + `}]}`
		}
		if _, err := io.WriteString(w, response); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := New("test-key", "test-model", server.URL+"/v1", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := any(provider).(domain.ToolProvider)
	if !ok {
		t.Fatal("provider does not support native tool turns")
	}
	history := &domain.ConversationHistory{SystemPrompt: "original-system", Messages: []domain.Message{
		{Role: roleUser, Content: "original-user"}, {Role: roleAssistant, Content: "original-assistant"}, {Role: roleUser, Content: "refinement"},
	}}
	before := append([]domain.Message(nil), history.Messages...)
	request := domain.ToolRequest{History: history, EnableTools: true}
	turn, err := tools.CompleteToolTurn(t.Context(), request)
	if err != nil || turn == nil {
		t.Fatalf("first turn = (%v, %v)", turn, err)
	}
	wantCall := domain.ToolCall{ID: "call-1", Name: "exec", Arguments: arguments}
	if turn.Content != "Inspecting" || !reflect.DeepEqual(turn.Calls, []domain.ToolCall{wantCall}) {
		t.Fatalf("first turn = %+v", turn)
	}
	first := <-requests
	var definitions []map[string]json.RawMessage
	if err := json.Unmarshal(first["tools"], &definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || string(definitions[0]["name"]) != `"exec"` {
		t.Fatalf("tool definitions = %+v", definitions)
	}
	var schema map[string]any
	if err := json.Unmarshal(definitions[0]["input_schema"], &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema, jsonObject(t, domain.ExecToolSchema())) {
		t.Fatalf("tool schema = %+v", schema)
	}
	request.Messages = []domain.ToolMessage{
		{Role: roleAssistant, Content: turn.Content, ToolCalls: turn.Calls},
		{Role: "tool", Content: `{"stdout":"/project"}`, ToolCallID: wantCall.ID},
	}
	turn, err = tools.CompleteToolTurn(t.Context(), request)
	if err != nil || turn == nil || turn.Content != final || len(turn.Calls) != 0 {
		t.Fatalf("final turn = (%+v, %v)", turn, err)
	}
	second := <-requests
	if string(second["system"]) != `"original-system"` {
		t.Fatalf("system = %s", second["system"])
	}
	var messages []struct {
		Role    string                       `json:"role"`
		Content []map[string]json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(second["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 5 || messages[0].Role != roleUser || string(messages[0].Content[0]["text"]) != `"original-user"` ||
		messages[3].Role != roleAssistant || string(messages[3].Content[1]["id"]) != `"call-1"` ||
		messages[4].Role != roleUser || string(messages[4].Content[0]["type"]) != `"tool_result"` ||
		string(messages[4].Content[0]["tool_use_id"]) != `"call-1"` {
		t.Fatalf("native exchange = %+v", messages)
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
