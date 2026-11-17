package prompt

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInputLocalizedHint(t *testing.T) {
	for _, tt := range []struct {
		language string
		hint     string
	}{
		{"ru", "(Нажмите Enter для подтверждения, Esc для отмены)"},
		{"en", "(Press Enter to confirm, Esc to cancel)"},
	} {
		t.Run(tt.language, func(t *testing.T) {
			t.Setenv("ZEV_LANG", tt.language)
			for _, secret := range []bool{false, true} {
				m := newInputModel("Enter custom base URL", secret)
				view := m.View()
				if !strings.Contains(view, tt.hint) {
					t.Errorf("View() lacks localized hint %q: %s", tt.hint, view)
				}
				if !strings.HasPrefix(view, "Enter custom base URL\n") {
					t.Errorf("View() changed caller-provided prompt: %s", view)
				}
			}
		})
	}
}

func TestMultilineLocalizedHint(t *testing.T) {
	t.Setenv("ZEV_LANG", "ru")
	m := newMultilineModel("Enter custom base URL")
	const value = "  user text {{literal}}\nкоманда\n"
	m.textArea.SetValue(value)
	view := m.View()
	if !strings.Contains(view, "Enter: новая строка, Ctrl+D: отправить или пропустить") {
		t.Errorf("View() lacks Russian multiline hint: %s", view)
	}
	if !strings.HasPrefix(view, "Enter custom base URL\n") {
		t.Errorf("View() changed caller-provided title: %s", view)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if got := requireMultilineModel(t, updated).value; got != value {
		t.Fatalf("submitted text = %q, want %q", got, value)
	}
	_, err := runMultilineContext(t.Context(), newMultilineModel("Log"), tea.WithInput(strings.NewReader("\x03")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	if err == nil || err.Error() != "cancelled" {
		t.Fatalf("multiline cancellation = %v, want literal cancelled", err)
	}
}

func TestRussianInputCancellationContract(t *testing.T) {
	t.Setenv("ZEV_LANG", "ru")
	_, err := inputContext(t.Context(), "Вопрос", tea.WithInput(strings.NewReader("\x03")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	if err == nil || err.Error() != "cancelled" {
		t.Fatalf("input cancellation = %v, want literal cancelled", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := InputContext(ctx, "Вопрос"); !errors.Is(err, context.Canceled) {
		t.Fatalf("input cancellation = %v, want context.Canceled", err)
	}
}
