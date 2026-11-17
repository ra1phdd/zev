package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	sdk "github.com/sashabaranov/go-openai"
)

func TestCompleteToolTurnValidatesEntireEnvelope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		suffix    string
		truncated bool
		wantErr   bool
		wantCause error
	}{
		{name: "complete"},
		{name: "trailing whitespace", suffix: " \r\n\t"},
		{name: "trailing JSON", suffix: ` {"secret":"response-secret"}`, wantErr: true},
		{name: "oversized suffix", suffix: strings.Repeat(" ", repository.MaxProviderResponseBytes), wantErr: true, wantCause: repository.ErrProviderResponseTooLarge},
		{name: "truncated HTTP body", truncated: true, wantErr: true, wantCause: io.ErrUnexpectedEOF},
	}
	for _, factory := range toolFactories() {
		t.Run(factory.name, func(t *testing.T) {
			t.Parallel()
			for _, enabled := range []bool{true, false} {
				t.Run(map[bool]string{true: "worker", false: "reviewer"}[enabled], func(t *testing.T) {
					t.Parallel()
					message := sdk.ChatCompletionMessage{Role: assistantRole, Content: `{"approved":true}`}
					reason := sdk.FinishReasonStop
					if enabled {
						message.ToolCalls = []sdk.ToolCall{{ID: firstCallID, Type: sdk.ToolTypeFunction, Function: sdk.FunctionCall{Name: execToolName, Arguments: toolArgs}}}
						reason = sdk.FinishReasonToolCalls
					}
					data, err := json.Marshal(sdk.ChatCompletionResponse{Choices: []sdk.ChatCompletionChoice{{FinishReason: reason, Message: message}}})
					if err != nil {
						t.Fatal(err)
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()
							server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
								w.Header().Set("Content-Type", "application/json")
								if tc.truncated {
									w.Header().Set("Content-Length", strconv.Itoa(len(data)+20))
								}
								if _, err := io.WriteString(w, string(data)+tc.suffix); err != nil {
									t.Errorf("write envelope: %v", err)
								}
							}))
							t.Cleanup(server.Close)
							provider := newToolProvider(t, factory, server)
							turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{EnableTools: enabled})
							if !tc.wantErr {
								if err != nil || turn == nil || len(turn.Calls) != len(message.ToolCalls) || turn.Content != message.Content {
									t.Fatalf("complete envelope = (%+v, %v)", turn, err)
								}
								return
							}
							if err == nil || turn != nil {
								t.Fatalf("incomplete envelope = (%+v, %v), want error and no turn", turn, err)
							}
							if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
								t.Errorf("error = %v, want cause %v", err, tc.wantCause)
							}
							if !strings.Contains(err.Error(), "invalid response") {
								t.Errorf("envelope error not classified as response failure: %v", err)
							}
							for _, secret := range []string{server.URL, toolKey, toolArgs, "response-secret"} {
								if strings.Contains(err.Error(), secret) {
									t.Errorf("error leaked response or request data: %v", err)
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestCompleteToolTurnCancellationDuringEnvelope(t *testing.T) {
	t.Parallel()
	for _, factory := range toolFactories() {
		t.Run(factory.name, func(t *testing.T) {
			t.Parallel()
			flushed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec","arguments":"{}"}}]}}]}`); err != nil {
					t.Errorf("write prefix: %v", err)
					return
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					t.Errorf("flush prefix: %v", err)
					return
				}
				close(flushed)
				<-r.Context().Done()
			}))
			t.Cleanup(server.Close)
			provider := newToolProvider(t, factory, server)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			type result struct {
				turn *domain.ToolTurn
				err  error
			}
			done := make(chan result, 1)
			go func() {
				turn, err := provider.CompleteToolTurn(ctx, domain.ToolRequest{EnableTools: true})
				done <- result{turn: turn, err: err}
			}()
			select {
			case <-flushed:
			case <-time.After(5 * time.Second):
				t.Fatal("response prefix was not flushed")
			}
			cancel()
			select {
			case got := <-done:
				if got.turn != nil || !errors.Is(got.err, context.Canceled) {
					t.Fatalf("canceled envelope = (%+v, %v), want cancellation and no turn", got.turn, got.err)
				}
				if strings.Contains(got.err.Error(), server.URL) {
					t.Errorf("cancellation error leaked URL: %v", got.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("body read did not stop on cancellation")
			}
		})
	}
}

func TestCompleteToolTurnPreservesHTTPErrorClassification(t *testing.T) {
	t.Parallel()
	for _, factory := range toolFactories() {
		t.Run(factory.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				if _, err := io.WriteString(w, "response-secret-not-json"); err != nil {
					t.Errorf("write error: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			provider := newToolProvider(t, factory, server)
			turn, err := provider.CompleteToolTurn(t.Context(), domain.ToolRequest{EnableTools: true})
			if turn != nil || err == nil || !strings.Contains(err.Error(), "authentication failed") {
				t.Fatalf("HTTP failure = (%+v, %v), want authentication failure", turn, err)
			}
			if strings.Contains(err.Error(), "response-secret") || strings.Contains(err.Error(), server.URL) {
				t.Errorf("HTTP error leaked body or URL: %v", err)
			}
		})
	}
}
