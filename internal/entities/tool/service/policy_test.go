package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestToolApprovalModesAndInvalidCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, mode, nameOverride, args string
		confirm                        bool
		missingConfirm                 bool
		checkErr                       error
		executions, checks             int
	}{
		{name: "approved manual", mode: modeManual, confirm: true, executions: 1},
		{name: "denied manual", mode: modeManual},
		{name: "headless manual", mode: modeManual, missingConfirm: true},
		{name: "bypass skips reviewer and static policy", mode: modeBypass, checkErr: errors.New("would be denied"), executions: 1},
		{name: "auto static deny", mode: modeAuto, checkErr: errors.New("outside project"), checks: 1},
		{name: "unknown tool", mode: modeBypass, nameOverride: "shell"},
		{name: "missing argument", mode: modeBypass, args: `{"command":"cat","reason":"inspect"}`},
		{name: "duplicate command", mode: modeBypass, args: `{"command":"ls","command":"cat","args":[],"reason":"inspect"}`},
		{name: "null argv", mode: modeBypass, args: `{"command":"pwd","args":null,"reason":"inspect"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := domain.ToolCall{ID: "invocation", Name: execToolName, Arguments: readArguments}
			if tc.nameOverride != "" {
				call.Name = tc.nameOverride
			}
			if tc.args != "" {
				call.Arguments = tc.args
			}
			p := &fixtureProvider{turns: []*domain.ToolTurn{{Calls: []domain.ToolCall{call}}, {Content: finalOptions}}}
			e := &fixtureExecutor{checkErr: tc.checkErr}
			settings := Settings{Mode: tc.mode, Root: fixtureProjectRoot, Executor: e}
			approvals := 0
			if !tc.missingConfirm {
				settings.Confirm = func(context.Context, domain.ExecRequest) (bool, error) { approvals++; return tc.confirm, nil }
			}
			s, err := New(p, settings)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{})
			if err != nil {
				t.Fatal(err)
			}
			if e.executed != tc.executions || e.checked != tc.checks {
				t.Fatalf("executed=%d checked=%d", e.executed, e.checked)
			}
			if tc.mode != modeManual && approvals != 0 {
				t.Fatal("non-manual mode prompted")
			}
			if tc.mode == modeManual && !tc.missingConfirm && approvals != 1 {
				t.Fatalf("approvals=%d", approvals)
			}
		})
	}
}

func TestToolLoopCallLimitAndDuplicateIDs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		count     int
		duplicate bool
		wantError bool
	}{{"maximum", 8, false, false}, {"over budget", 9, false, true}, {"duplicate IDs", 2, true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := make([]domain.ToolCall, tc.count)
			for i := range calls {
				id := fmt.Sprintf("call-%d", i)
				if tc.duplicate {
					id = "same"
				}
				calls[i] = domain.ToolCall{ID: id, Name: execToolName, Arguments: readArguments}
			}
			p := &fixtureProvider{turns: []*domain.ToolTurn{{Calls: calls}, {Content: finalOptions}}}
			e := &fixtureExecutor{}
			s, err := New(p, Settings{Mode: modeBypass, Root: fixtureProjectRoot, Executor: e})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{})
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			if tc.wantError && e.executed != 0 {
				t.Fatalf("malformed batch executed %d", e.executed)
			}
			if !tc.wantError && (e.executed != 8 || p.requests[1].EnableTools) {
				t.Fatalf("budget not enforced %+v", p.requests)
			}
		})
	}
}

func TestEveryAutoCallGetsAnIndependentReview(t *testing.T) {
	t.Parallel()
	calls := []domain.ToolCall{{ID: "first", Name: execToolName, Arguments: readArguments}, {ID: "second", Name: execToolName, Arguments: readArguments}}
	p := &fixtureProvider{turns: []*domain.ToolTurn{{Calls: calls}, {Content: `{"allowed":true,"reason":"read only"}`}, {Content: `{"allowed":false,"reason":"not necessary"}`}, {Content: finalOptions}}}
	e := &fixtureExecutor{}
	s, err := New(p, Settings{Mode: modeAuto, Root: fixtureProjectRoot, Executor: e})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{})
	if err != nil {
		t.Fatal(err)
	}
	if e.executed != 1 || e.checked != 2 || len(p.requests) != 4 || p.requests[1].EnableTools || p.requests[2].EnableTools {
		t.Fatalf("review bypass executed=%d checked=%d requests=%d", e.executed, e.checked, len(p.requests))
	}
	if !strings.Contains(p.requests[3].Messages[2].Content, "not necessary") {
		t.Fatal("reviewer denial not returned to primary")
	}
}

func TestCanceledToolSessionDoesNotCallProvider(t *testing.T) {
	t.Parallel()
	p := &fixtureProvider{}
	e := &fixtureExecutor{}
	s, err := New(p, Settings{Mode: modeBypass, Root: fixtureProjectRoot, Executor: e})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.GetOptionsWithHistory(ctx, &domain.ConversationHistory{})
	if !errors.Is(err, context.Canceled) || len(p.requests) != 0 || e.executed != 0 {
		t.Fatalf("cancel error=%v requests=%d executions=%d", err, len(p.requests), e.executed)
	}
}
