package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestCompleteToolTurnParallelCallsWithoutIDs(t *testing.T) {
	t.Parallel()
	requests := make(chan map[string]json.RawMessage, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- body
		response := contentResponse("STOP", `{"thought":true,"text":"private reasoning"},{"functionCall":{"name":"exec","args":{"command":"pwd","args":[],"reason":"context"}},"thoughtSignature":"sig-1"},{"functionCall":{"name":"exec","args":{"command":"uname","args":[],"reason":"context"}},"thoughtSignature":"sig-2"}`)
		if _, err := io.WriteString(w, response); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := New("test-key", "test-model", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request := domain.ToolRequest{EnableTools: true, History: &domain.ConversationHistory{Messages: []domain.Message{{Role: geminiRoleUser, Content: "inspect"}}}}
	first, err := provider.CompleteToolTurn(t.Context(), request)
	if err != nil || first == nil || len(first.Calls) != 2 || first.Content != "" {
		t.Fatalf("parallel turn = (%+v, %v)", first, err)
	}
	if first.Calls[0].ID == "" || first.Calls[1].ID == "" || first.Calls[0].ID == first.Calls[1].ID ||
		first.Calls[0].Signature != "sig-1" || first.Calls[1].Signature != "sig-2" {
		t.Fatalf("call correlation/signatures = %+v", first.Calls)
	}
	<-requests
	request.Messages = []domain.ToolMessage{
		{Role: geminiRoleAssistant, ToolCalls: first.Calls},
		{Role: geminiRoleTool, ToolCallID: first.Calls[0].ID, Content: "plain output"},
		{Role: geminiRoleTool, ToolCallID: first.Calls[1].ID, Content: `{"stdout":"system"}`},
	}
	second, err := provider.CompleteToolTurn(t.Context(), request)
	if err != nil || second == nil || len(second.Calls) != 2 {
		t.Fatalf("second turn = (%+v, %v)", second, err)
	}
	if second.Calls[0].ID == first.Calls[0].ID || second.Calls[1].ID == first.Calls[1].ID {
		t.Fatal("synthesized call IDs were reused across turns")
	}
	body := <-requests
	var contents []struct {
		Role  string                       `json:"role"`
		Parts []map[string]json.RawMessage `json:"parts"`
	}
	if err := json.Unmarshal(body[geminiJSONContents], &contents); err != nil {
		t.Fatal(err)
	}
	if len(contents) != 3 || contents[1].Role != geminiRoleModel || contents[2].Role != geminiRoleUser || len(contents[2].Parts) != 2 {
		t.Fatalf("parallel results must share a user turn: %s", body[geminiJSONContents])
	}
	for i, call := range first.Calls {
		var sent struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(contents[1].Parts[i]["functionCall"], &sent); err != nil {
			t.Fatal(err)
		}
		var signature string
		if err := json.Unmarshal(contents[1].Parts[i]["thoughtSignature"], &signature); err != nil {
			t.Fatal(err)
		}
		var result struct {
			ID       string            `json:"id"`
			Name     string            `json:"name"`
			Response map[string]string `json:"response"`
		}
		if err := json.Unmarshal(contents[2].Parts[i]["functionResponse"], &result); err != nil {
			t.Fatal(err)
		}
		if sent.ID != call.ID || result.ID != call.ID || sent.Name != geminiExecName || result.Name != geminiExecName || signature != call.Signature {
			t.Fatalf("parallel call %d not correlated: call=%+v result=%+v signature=%s", i, sent, result, signature)
		}
		if i == 0 && result.Response["output"] != "plain output" || i == 1 && result.Response["stdout"] != "system" {
			t.Fatalf("parallel result %d = %+v", i, result.Response)
		}
	}
}
