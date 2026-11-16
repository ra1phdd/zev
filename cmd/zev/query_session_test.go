package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/spf13/cobra"
)

const (
	queryTestPrompt           = "show 100% of files %s"
	queryTestRefinement       = "remove selected files"
	queryTestDangerousCommand = "rm -rf /"
)

type queryProviderResult struct {
	response *domain.OptionsResponse
	err      error
}

type queryFlowProvider struct {
	results   []queryProviderResult
	histories []domain.ConversationHistory
}

func (p *queryFlowProvider) GetOptions(_ context.Context, _, _ string) (*domain.OptionsResponse, error) {
	return nil, errors.New("unexpected history-free provider call")
}

func (p *queryFlowProvider) GetOptionsWithHistory(_ context.Context, history *domain.ConversationHistory) (*domain.OptionsResponse, error) {
	p.histories = append(p.histories, domain.ConversationHistory{
		SystemPrompt: history.SystemPrompt,
		Messages:     append([]domain.Message(nil), history.Messages...),
	})
	if len(p.results) == 0 {
		return nil, errors.New("unexpected extra request")
	}
	result := p.results[0]
	p.results = p.results[1:]
	return result.response, result.err
}

func (p *queryFlowProvider) Name() string  { return "test" }
func (p *queryFlowProvider) Model() string { return "test-model" }

func newTestQuerySession(t *testing.T, provider *queryFlowProvider) *querySession {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return newQuerySession(queryservice.New(provider), cmd, queryTestPrompt)
}

func TestQueryRefinementUsesCommandContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	s := newQuerySession(queryservice.New(&queryFlowProvider{}), cmd, queryTestPrompt)
	_, err := s.refine(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("refine() error = %v, want context.Canceled", err)
	}
}

func TestQuerySelectionUsesCommandContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	s := newQuerySession(queryservice.New(&queryFlowProvider{}), cmd, queryTestPrompt)
	_, err := s.selectItem(list.SelectOptions{Items: []list.Item{list.NewSimpleItem("item", "")}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("selectItem() error = %v, want context.Canceled", err)
	}
}

func TestQueryConversationPreservesPromptAndEnvironmentSplit(t *testing.T) {
	t.Parallel()
	provider := &queryFlowProvider{results: []queryProviderResult{
		{response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "ls"}}}},
		{response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: queryTestDangerousCommand}}}},
	}}
	s := newTestQuerySession(t, provider)
	if err := s.start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if ready, err := s.acceptResponse(); err != nil || !ready {
		t.Fatalf("acceptResponse() = %v, %v", ready, err)
	}
	s.input = func(string) (string, error) { return queryTestRefinement, nil }
	selections := 0
	s.selectItem = func(options list.SelectOptions) (list.Item, error) {
		selections++
		if selections == 1 {
			return options.Items[len(options.Items)-1], nil
		}
		return options.Items[0], nil
	}
	selected, index, err := s.selectCommand(t.Context(), executionModeInject)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || !selected.IsDangerous || selected.DangerousExplanation == "" || index != 0 {
		t.Fatalf("refined selection = %+v, index %d; want local safety warning", selected, index)
	}
	if len(provider.histories) != 2 {
		t.Fatalf("requests = %d, want 2", len(provider.histories))
	}
	initial, refined := provider.histories[0], provider.histories[1]
	if len(initial.Messages) != 1 || initial.Messages[0].Content != queryTestPrompt {
		t.Fatalf("initial messages = %+v; want literal user prompt only", initial.Messages)
	}
	if !strings.Contains(initial.SystemPrompt, "OS:") || strings.Contains(initial.SystemPrompt, queryTestPrompt) {
		t.Errorf("system prompt must contain environment, not user prompt: %q", initial.SystemPrompt)
	}
	if refined.SystemPrompt != initial.SystemPrompt || len(refined.Messages) != 3 {
		t.Fatalf("refinement history = %+v; want same system prompt and user/assistant/user", refined)
	}
	if refined.Messages[1].Role != "assistant" || refined.Messages[2].Content != queryTestRefinement {
		t.Errorf("refined messages = %+v", refined.Messages)
	}
	if s.userPrompt != queryTestRefinement {
		t.Errorf("saved prompt = %q, want latest refinement", s.userPrompt)
	}
}

func TestQueryInitialAndClarificationUseSafetyService(t *testing.T) {
	t.Parallel()
	for _, clarify := range []bool{false, true} {
		name := "initial"
		if clarify {
			name = "clarification"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			provider := &queryFlowProvider{}
			if clarify {
				provider.results = append(provider.results, queryProviderResult{err: domain.ErrNoResponse})
			}
			provider.results = append(provider.results, queryProviderResult{response: &domain.OptionsResponse{
				IsValid: true, Commands: []domain.Command{{Command: queryTestDangerousCommand}},
			}})
			s := newTestQuerySession(t, provider)
			s.input = func(string) (string, error) { return queryTestRefinement, nil }
			if err := s.start(t.Context()); err != nil {
				t.Fatal(err)
			}
			selected, _, err := s.selectCommand(t.Context(), executionModeExecute)
			if err != nil || selected == nil || !selected.IsDangerous || selected.DangerousExplanation == "" {
				t.Fatalf("auto-selection = %+v, error %v; want warning without executing command", selected, err)
			}
			if clarify && (len(provider.histories) != 2 || len(provider.histories[1].Messages) != 2) {
				t.Errorf("clarification histories = %+v", provider.histories)
			}
		})
	}
}

func TestQueryDiagnosticOutputFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		response    *domain.OptionsResponse
		providerErr error
	}{
		{name: "invalid request", response: &domain.OptionsResponse{ExplanationIfNotValid: "invalid", Commands: []domain.Command{{Command: queryRefineMarker}}}},
		{name: "clarification warning", providerErr: domain.ErrNoResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider := &queryFlowProvider{results: []queryProviderResult{{response: tt.response, err: tt.providerErr}}}
			s := newTestQuerySession(t, provider)
			outputErr := errors.New("diagnostic output unavailable")
			s.cmd.SetErr(failingOutput{err: outputErr})
			s.input = func(string) (string, error) {
				t.Fatal("input must not be requested after output failure")
				return "", nil
			}
			err := s.start(t.Context())
			if tt.providerErr == nil {
				s.cmd.SetOut(failingOutput{err: outputErr})
				_, err = s.acceptResponse()
			}
			if !errors.Is(err, outputErr) {
				t.Fatalf("query error = %v, want diagnostic output error", err)
			}
		})
	}
}

func TestQuerySelectionRejectsUnexpectedItems(t *testing.T) {
	t.Parallel()
	for _, item := range []list.Item{nil, list.NewSimpleItem("wrong", "type"), list.CommandItem{}} {
		t.Run("unexpected item", func(t *testing.T) {
			t.Parallel()
			s := newTestQuerySession(t, &queryFlowProvider{})
			s.response = &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "ls"}}}
			s.selectItem = func(list.SelectOptions) (list.Item, error) { return item, nil }
			if _, _, err := s.selectCommand(t.Context(), executionModeInject); err == nil {
				t.Fatal("selectCommand() accepted an unexpected item")
			}
		})
	}
}

func TestQueryWarningOutput(t *testing.T) {
	t.Parallel()
	command := &domain.Command{IsDangerous: true, DangerousExplanation: "local warning"}
	var out bytes.Buffer
	if err := writeQueryWarning(&out, command); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), command.DangerousExplanation) {
		t.Errorf("warning = %q", out.String())
	}
	outputErr := errors.New("warning output unavailable")
	if err := writeQueryWarning(failingOutput{err: outputErr}, command); !errors.Is(err, outputErr) {
		t.Errorf("writeQueryWarning() error = %v, want output error", err)
	}
}
