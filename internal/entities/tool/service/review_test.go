package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestFailedReviewerDoesNotAuthorizeExecution(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		review  *domain.ToolTurn
		network bool
	}{
		{name: "network failure", network: true},
		{name: "empty reviewer", review: nil},
		{name: "reviewer requests tool", review: &domain.ToolTurn{Calls: []domain.ToolCall{{ID: "review-call", Name: execToolName, Arguments: readArguments}}}},
		{name: "reviewer response oversized", review: &domain.ToolTurn{Content: strings.Repeat("x", 4097)}},
		{name: "verdict case mismatch", review: &domain.ToolTurn{Content: `{"Allowed":true,"reason":"wrong field"}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turns := []*domain.ToolTurn{{Calls: []domain.ToolCall{{ID: "primary", Name: execToolName, Arguments: readArguments}}}, tc.review, {Content: finalOptions}}
			p := &fixtureProvider{turns: turns}
			if tc.network {
				p.err = errors.New("review unavailable")
				p.failAt = 2
				p.turns = []*domain.ToolTurn{turns[0], turns[2]}
			}
			e := &fixtureExecutor{}
			s, err := New(p, Settings{Mode: modeAuto, Root: fixtureProjectRoot, Executor: e})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{})
			if err != nil {
				t.Fatal(err)
			}
			if e.executed != 0 || !strings.Contains(p.requests[2].Messages[1].Content, `"denied":true`) {
				t.Fatal("failed reviewer authorized tool")
			}
		})
	}
}
