package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const fixtureProjectRoot = "/project"

const (
	finalOptions   = `{"is_valid":true,"commands":[{"command":"echo done","short_explanation":"done","is_dangerous":false}]}`
	readArguments  = `{"command":"cat","args":["README.md"],"reason":"Understand the project"}`
	readToolCallID = "read"
)

type fixtureProvider struct {
	requests []domain.ToolRequest
	turns    []*domain.ToolTurn
	err      error
	failAt   int
}

func (p *fixtureProvider) Name() string  { return "fixture" }
func (p *fixtureProvider) Model() string { return "fixture-model" }
func (p *fixtureProvider) GetOptions(context.Context, string, string) (*domain.OptionsResponse, error) {
	return nil, errors.New("unexpected legacy request")
}

func (p *fixtureProvider) GetOptionsWithHistory(context.Context, *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	return nil, errors.New("unexpected legacy history")
}

func (p *fixtureProvider) CompleteToolTurn(_ context.Context, r domain.ToolRequest) (*domain.ToolTurn, error) {
	p.requests = append(p.requests, r)
	if p.err != nil && (p.failAt == 0 || len(p.requests) == p.failAt) {
		return nil, p.err
	}
	if len(p.turns) == 0 {
		return nil, errors.New("unexpected request")
	}
	turn := p.turns[0]
	p.turns = p.turns[1:]
	return turn, nil
}

type fixtureExecutor struct {
	checked    int
	executed   int
	restricted bool
	checkErr   error
	output     string
}

func (e *fixtureExecutor) CheckReadOnly(domain.ExecRequest) error { e.checked++; return e.checkErr }
func (e *fixtureExecutor) Execute(_ context.Context, _ domain.ExecRequest, restricted bool) domain.ToolResult {
	e.executed++
	e.restricted = restricted
	output := e.output
	if output == "" {
		output = "project details"
	}
	return domain.ToolResult{Output: output, ExitCode: 0}
}

func TestServiceExposesUnderlyingToolProviderForIsolatedReviews(t *testing.T) {
	t.Parallel()
	provider := &fixtureProvider{turns: []*domain.ToolTurn{{Content: `{"safe":true,"reason":"read only"}`}}}
	executor := &fixtureExecutor{}
	service, err := New(provider, Settings{Mode: modeAuto, Root: fixtureProjectRoot, Executor: executor})
	if err != nil {
		t.Fatal(err)
	}

	toolProvider := domain.ToolProvider(service)
	request := domain.ToolRequest{History: &domain.ConversationHistory{SystemPrompt: "isolated review"}}
	got, err := toolProvider.CompleteToolTurn(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Content != `{"safe":true,"reason":"read only"}` || len(provider.requests) != 1 || provider.requests[0].History.SystemPrompt != "isolated review" {
		t.Fatalf("forwarded tool turn = (%+v, requests=%+v)", got, provider.requests)
	}
}

func TestManualDenialReturnsToPrimaryWithoutExecuting(t *testing.T) {
	t.Parallel()
	p := &fixtureProvider{turns: []*domain.ToolTurn{{Calls: []domain.ToolCall{{ID: "call-1", Name: execToolName, Arguments: readArguments}}}, {Content: finalOptions}}}
	e := &fixtureExecutor{}
	confirmations := 0
	s, err := New(p, Settings{Mode: modeManual, Root: fixtureProjectRoot, Executor: e, Confirm: func(context.Context, domain.ExecRequest) (bool, error) { confirmations++; return false, nil }})
	if err != nil {
		t.Fatal(err)
	}
	h := &domain.ConversationHistory{SystemPrompt: "original", Messages: []domain.Message{{Role: "user", Content: "help"}}}
	result, err := s.GetOptionsWithHistory(t.Context(), h)
	if err != nil {
		t.Fatal(err)
	}
	if result.Commands[0].Command != "echo done" || confirmations != 1 || e.executed != 0 {
		t.Fatalf("result=%+v confirmations=%d executions=%d", result, confirmations, e.executed)
	}
	if h.SystemPrompt != "original" || len(h.Messages) != 1 {
		t.Fatal("caller history mutated")
	}
	messages := p.requests[1].Messages
	if len(messages) != 2 || messages[1].ToolCallID != "call-1" {
		t.Fatalf("tool exchange=%+v", messages)
	}
	var denied domain.ToolResult
	if err := json.Unmarshal([]byte(messages[1].Content), &denied); err != nil {
		t.Fatal(err)
	}
	if !denied.Denied || denied.Reason == "" {
		t.Fatalf("missing denial %+v", denied)
	}
}

func TestAutoReviewIsIsolatedAndRequiredBeforeEveryExecution(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, review string
		allow        bool
	}{{"allow", `{"allowed":true,"reason":"read only"}`, true}, {"deny", `{"allowed":false,"reason":"unsafe"}`, false}, {"missing verdict", `{"reason":"unknown"}`, false}, {"invalid", `not JSON`, false}, {"duplicate", `{"allowed":false,"allowed":true,"reason":"oops"}`, false}, {"unknown", `{"allowed":true,"reason":"ok","extra":true}`, false}} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fixtureProvider{turns: []*domain.ToolTurn{{Calls: []domain.ToolCall{{ID: readToolCallID, Name: execToolName, Arguments: readArguments}}}, {Content: tc.review}, {Content: finalOptions}}}
			e := &fixtureExecutor{}
			s, err := New(p, Settings{Mode: modeAuto, Root: fixtureProjectRoot, Executor: e})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{SystemPrompt: "private primary instructions", Messages: []domain.Message{{Role: "user", Content: "private conversation"}}})
			if err != nil {
				t.Fatal(err)
			}
			if (e.executed == 1) != tc.allow || (tc.allow && !e.restricted) {
				t.Fatalf("executed=%d restricted=%t", e.executed, e.restricted)
			}
			review := p.requests[1]
			if review.EnableTools || len(review.Messages) != 0 || review.History.SystemPrompt != reviewInstructions || len(review.History.Messages) != 1 {
				t.Fatalf("review not isolated %+v", review)
			}
			var envelope struct {
				Root    string   `json:"root"`
				Command string   `json:"command"`
				Args    []string `json:"args"`
			}
			if err := json.Unmarshal([]byte(review.History.Messages[0].Content), &envelope); err != nil {
				t.Fatalf("review payload is not valid JSON: %v", err)
			}
			if envelope.Root != fixtureProjectRoot || envelope.Command != "cat" || len(envelope.Args) != 1 || envelope.Args[0] != "README.md" {
				t.Fatalf("review payload = %+v", envelope)
			}
			if strings.Contains(review.History.Messages[0].Content, "Understand the project") {
				t.Fatal("review payload included agent-authored reason")
			}
			if strings.Contains(review.History.Messages[0].Content, "private conversation") || strings.Contains(review.History.Messages[0].Content, "private primary instructions") {
				t.Fatal("review payload leaked primary conversation")
			}
		})
	}
}
