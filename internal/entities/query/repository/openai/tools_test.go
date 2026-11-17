package openai_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository/ollama"
	"github.com/ra1phdd/zev/internal/entities/query/repository/openai"
	"github.com/ra1phdd/zev/internal/entities/query/repository/opencode"
	sdk "github.com/sashabaranov/go-openai"
)

const (
	toolModel      = "fixture-model"
	toolKey        = "fixture-key"
	toolArgs       = `{"command":"git","args":["status","--short"],"reason":"inspect worktree"}`
	toolFinal      = `{"options":[{"command":"git status","explanation":"Show status","isDangerous":false}]}`
	assistantRole  = "assistant"
	firstCallID    = "call_1"
	secondCallID   = "call_2"
	execToolName   = "exec"
	toolRole       = "tool"
	partialContent = "partial"
)

type providerFactory struct {
	name        string
	newProvider func(string, *http.Client) (domain.InferenceProvider, error)
}

func toolFactories() []providerFactory {
	return []providerFactory{
		{name: "openai", newProvider: func(url string, client *http.Client) (domain.InferenceProvider, error) {
			return openai.New(toolKey, toolModel, url, client)
		}},
		{name: "ollama", newProvider: func(url string, client *http.Client) (domain.InferenceProvider, error) {
			return ollama.New(url, toolModel, client)
		}},
		{name: "opencode", newProvider: func(url string, client *http.Client) (domain.InferenceProvider, error) {
			return opencode.New(toolKey, toolModel, url, client)
		}},
	}
}

func TestCompleteToolTurnExchange(t *testing.T) {
	t.Parallel()
	for _, factory := range toolFactories() {
		t.Run(factory.name, func(t *testing.T) {
			t.Parallel()
			requests := make(chan sdk.ChatCompletionRequest, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
					t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
				}
				var request sdk.ChatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- request
				w.Header().Set("Content-Type", "application/json")
				response := `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"Gathering context","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec","arguments":` + quoteJSON(t, toolArgs) + `}},{"id":"call_2","type":"function","function":{"name":"exec","arguments":` + quoteJSON(t, toolArgs) + `}}]}}]}`
				if len(request.Messages) > 3 {
					response = `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":` + quoteJSON(t, toolFinal) + `}}]}`
				}
				if _, err := io.WriteString(w, response); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			provider := newToolProvider(t, factory, server)
			history := &domain.ConversationHistory{SystemPrompt: "trusted system", Messages: []domain.Message{{Role: "user", Content: "original"}, {Role: assistantRole, Content: "earlier"}}}
			request := domain.ToolRequest{History: history, EnableTools: true}
			turn, err := provider.CompleteToolTurn(t.Context(), request)
			if err != nil {
				t.Fatalf("first turn: %v", err)
			}
			wantCalls := []domain.ToolCall{{ID: firstCallID, Name: execToolName, Arguments: toolArgs}, {ID: secondCallID, Name: execToolName, Arguments: toolArgs}}
			if turn.Content != "Gathering context" || !reflect.DeepEqual(turn.Calls, wantCalls) {
				t.Fatalf("first turn = %+v", turn)
			}
			first := <-requests
			assertToolRequest(t, first)
			if len(first.Messages) != 3 || first.Messages[0].Content != history.SystemPrompt || first.Messages[1].Content != "original" || first.Messages[2].Content != "earlier" {
				t.Fatalf("original messages lost: %+v", first.Messages)
			}
			request.Messages = []domain.ToolMessage{{Role: assistantRole, Content: turn.Content, ToolCalls: turn.Calls}, {Role: toolRole, Content: "untrusted output 1", ToolCallID: firstCallID}, {Role: toolRole, Content: "untrusted output 2", ToolCallID: secondCallID}}
			turn, err = provider.CompleteToolTurn(t.Context(), request)
			if err != nil {
				t.Fatalf("final turn: %v", err)
			}
			if turn.Content != toolFinal || len(turn.Calls) != 0 {
				t.Fatalf("final turn = %+v", turn)
			}
			second := <-requests
			assertToolRequest(t, second)
			if len(second.Messages) != 6 || len(second.Messages[3].ToolCalls) != 2 || second.Messages[3].ToolCalls[0].ID != firstCallID || second.Messages[3].ToolCalls[1].Function.Arguments != toolArgs || second.Messages[4].ToolCallID != firstCallID || second.Messages[5].ToolCallID != secondCallID || second.Messages[5].Content != "untrusted output 2" {
				t.Fatalf("tool correlation lost: %+v", second.Messages)
			}
			if len(history.Messages) != 2 {
				t.Fatal("original history mutated")
			}
		})
	}
}

func TestFinalToolTurnReplaysCallsWhenDisabled(t *testing.T) {
	t.Parallel()
	requests := make(chan sdk.ChatCompletionRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request sdk.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- request
		var response string
		if len(request.Messages) == 0 {
			response = `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"inspect","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec","arguments":` + quoteJSON(t, toolArgs) + `}}]}}]}`
		} else {
			response = `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":` + quoteJSON(t, toolFinal) + `}}]}`
		}
		if _, err := io.WriteString(w, response); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := openai.New(toolKey, toolModel, server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	tools := domain.ToolProvider(provider)
	first, err := tools.CompleteToolTurn(t.Context(), domain.ToolRequest{EnableTools: true})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if len(first.Calls) != 1 {
		t.Fatalf("first calls = %+v", first.Calls)
	}
	final, err := tools.CompleteToolTurn(t.Context(), domain.ToolRequest{
		EnableTools: false,
		Messages: []domain.ToolMessage{
			{Role: "assistant", Content: first.Content, ToolCalls: first.Calls},
			{Role: "tool", Content: "untrusted result", ToolCallID: first.Calls[0].ID},
		},
	})
	if err != nil || final == nil || final.Content != toolFinal || len(final.Calls) != 0 {
		t.Fatalf("final turn = (%+v, %v)", final, err)
	}
	firstRequest := <-requests
	if len(firstRequest.Tools) != 1 {
		t.Fatalf("first request tools = %+v", firstRequest.Tools)
	}
	secondRequest := <-requests
	if len(secondRequest.Tools) != 0 {
		t.Fatalf("final request advertised tools: %+v", secondRequest.Tools)
	}
	if len(secondRequest.Messages) != 2 || len(secondRequest.Messages[0].ToolCalls) != 1 || secondRequest.Messages[0].ToolCalls[0].ID != "call_1" || secondRequest.Messages[1].ToolCallID != "call_1" {
		t.Fatalf("final request lost historical tool exchange: %+v", secondRequest.Messages)
	}
}

func TestCompleteToolTurnOmitsToolsWhenDisabled(t *testing.T) {
	t.Parallel()
	for _, factory := range toolFactories() {
		t.Run(factory.name, func(t *testing.T) {
			t.Parallel()
			var body map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": `{"review":true}`}}}}); err != nil {
					t.Errorf("write review: %v", err)
				}
			}))
			defer server.Close()
			provider := newToolProvider(t, factory, server)
			turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{EnableTools: false})
			if err != nil || turn == nil || turn.Content != `{"review":true}` || len(turn.Calls) != 0 {
				t.Fatalf("disabled turn = (%+v, %v)", turn, err)
			}
			if _, exists := body["tools"]; exists {
				t.Fatal("disabled request advertised tools")
			}
			if _, exists := body["response_format"]; exists {
				t.Fatal("disabled request forced options response format")
			}
		})
	}
}

func TestCompleteToolTurnRejectsUnsafeResponses(t *testing.T) {
	t.Parallel()
	validCall := sdk.ToolCall{ID: firstCallID, Type: sdk.ToolTypeFunction, Function: sdk.FunctionCall{Name: execToolName, Arguments: toolArgs}}
	cases := []struct {
		name     string
		reason   sdk.FinishReason
		calls    []sdk.ToolCall
		enabled  bool
		role     string
		content  string
		trailing string
	}{
		{name: "truncated valid call", reason: sdk.FinishReasonLength, calls: []sdk.ToolCall{validCall}, enabled: true},
		{name: "truncated final", reason: sdk.FinishReasonLength, content: partialContent, enabled: true},
		{name: "missing finish reason", calls: []sdk.ToolCall{validCall}, enabled: true},
		{name: "unknown finish reason", reason: "unknown", content: partialContent, enabled: true},
		{name: "filtered final", reason: sdk.FinishReasonContentFilter, content: partialContent, enabled: true},
		{name: "calls with stop", reason: sdk.FinishReasonStop, calls: []sdk.ToolCall{validCall}, enabled: true},
		{name: "tool stop without calls", reason: sdk.FinishReasonToolCalls, enabled: true},
		{name: "calls disabled", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{validCall}},
		{name: "duplicate call IDs", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{validCall, validCall}, enabled: true},
		{name: "empty ID", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{{Type: sdk.ToolTypeFunction, Function: validCall.Function}}, enabled: true},
		{name: "empty name", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{{ID: firstCallID, Type: sdk.ToolTypeFunction, Function: sdk.FunctionCall{Arguments: toolArgs}}}, enabled: true},
		{name: "wrong call type", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{{ID: firstCallID, Type: "custom", Function: validCall.Function}}, enabled: true},
		{name: "truncated arguments", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{{ID: firstCallID, Type: sdk.ToolTypeFunction, Function: sdk.FunctionCall{Name: execToolName, Arguments: `{"command":`}}}, enabled: true},
		{name: "non object arguments", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{{ID: firstCallID, Type: sdk.ToolTypeFunction, Function: sdk.FunctionCall{Name: execToolName, Arguments: `[]`}}}, enabled: true},
		{name: "wrong role", role: "user", reason: sdk.FinishReasonToolCalls, calls: []sdk.ToolCall{validCall}, enabled: true},
		{name: "empty response", reason: sdk.FinishReasonStop, enabled: true},
	}
	for _, factory := range toolFactories() {
		t.Run(factory.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						role := tc.role
						if role == "" {
							role = sdk.ChatMessageRoleAssistant
						}
						response := sdk.ChatCompletionResponse{Choices: []sdk.ChatCompletionChoice{{FinishReason: tc.reason, Message: sdk.ChatCompletionMessage{Role: role, Content: tc.content, ToolCalls: tc.calls}}}}
						if err := json.NewEncoder(w).Encode(response); err != nil {
							t.Errorf("encode response: %v", err)
						}
						if _, err := io.WriteString(w, tc.trailing); err != nil {
							t.Errorf("write trailing bytes: %v", err)
						}
					}))
					t.Cleanup(server.Close)
					provider := newToolProvider(t, factory, server)
					turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{EnableTools: tc.enabled})
					if err == nil || turn != nil {
						t.Fatalf("unsafe response = (%+v, %v), want error and no turn", turn, err)
					}
				})
			}
		})
	}
}

func TestCompleteToolTurnNetworkError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	client := &http.Client{}
	provider, err := openai.New(toolKey, toolModel, url, client)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{EnableTools: true})
	if err == nil || turn != nil {
		t.Fatalf("network failure = (%+v, %v), want error and no turn", turn, err)
	}
}

func newToolProvider(t *testing.T, factory providerFactory, server *httptest.Server) domain.ToolProvider {
	t.Helper()
	provider, err := factory.newProvider(server.URL, server.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	toolProvider, ok := provider.(domain.ToolProvider)
	if !ok {
		t.Fatalf("%s does not support native tools", factory.name)
	}
	return toolProvider
}

func assertToolRequest(t *testing.T, request sdk.ChatCompletionRequest) {
	t.Helper()
	if request.Model != toolModel || request.Stream || request.ResponseFormat != nil {
		t.Errorf("unexpected request mode: %+v", request)
	}
	if len(request.Tools) != 1 || request.Tools[0].Type != sdk.ToolTypeFunction || request.Tools[0].Function.Name != "exec" {
		t.Fatalf("tools = %+v", request.Tools)
	}
	got, err := json.Marshal(request.Tools[0].Function.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(domain.ExecToolSchema())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("tool schema = %s, want %s", got, want)
	}
	if request.Tools[0].Function.Description == "" {
		t.Error("missing tool description")
	}
}

func quoteJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
