package prompt

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const approvalTestPrompt = "Approve?"

func TestConfirmContextEOFReturnsWithoutApproval(t *testing.T) {
	t.Parallel()

	approved, err := ConfirmContext(t.Context(), ApprovalOptions{
		Prompt: approvalTestPrompt,
		Input:  strings.NewReader(""),
		Output: &strings.Builder{},
	})
	if approved || err == nil {
		t.Fatalf("approval=%t error=%v, want an EOF cancellation", approved, err)
	}
}

func TestConfirmContextCancellationNeverApproves(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	approved, err := ConfirmContext(ctx, ApprovalOptions{Prompt: approvalTestPrompt})
	if approved || !errors.Is(err, context.Canceled) {
		t.Fatalf("approval=%t error=%v", approved, err)
	}
}

func TestApprovalModelUsesYNUILanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		key       tea.KeyMsg
		approved  bool
		denied    bool
		cancelled bool
	}{
		{name: "y approves", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, approved: true},
		{name: "n denies", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, denied: true},
		{name: "enter approves", key: tea.KeyMsg{Type: tea.KeyEnter}, approved: true},
		{name: "escape cancels", key: tea.KeyMsg{Type: tea.KeyEsc}, cancelled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := approvalModel{prompt: approvalTestPrompt}
			updated, cmd := m.Update(tt.key)
			got, ok := updated.(approvalModel)
			if !ok {
				t.Fatalf("unexpected model type %T", updated)
			}
			if got.approved != tt.approved || got.denied != tt.denied || got.cancelled != tt.cancelled {
				t.Fatalf("state = %+v, want approved=%t denied=%t cancelled=%t", got, tt.approved, tt.denied, tt.cancelled)
			}
			if cmd == nil {
				t.Fatal("expected quit command")
			}
		})
	}
}

func TestApprovalModelCancelsOnCtrlC(t *testing.T) {
	t.Parallel()

	model := approvalModel{prompt: approvalTestPrompt}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got, ok := updated.(approvalModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !got.cancelled || got.approved || got.denied || cmd == nil {
		t.Fatalf("Ctrl+C did not cancel approval: %+v, cmd=%v", got, cmd)
	}
}

func TestConfirmContextAcceptsYAndN(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		input    string
		approved bool
	}{
		{name: "y", input: "y", approved: true},
		{name: "n", input: "n", approved: false},
		{name: "enter", input: "\r", approved: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			approved, err := confirmContext(t.Context(), ApprovalOptions{Prompt: approvalTestPrompt, Input: strings.NewReader(tt.input), Output: &strings.Builder{}}, tea.WithoutSignalHandler())
			if err != nil {
				t.Fatalf("confirmContext() error = %v", err)
			}
			if approved != tt.approved {
				t.Fatalf("approved = %t, want %t", approved, tt.approved)
			}
		})
	}
}
