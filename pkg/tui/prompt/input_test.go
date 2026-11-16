package prompt

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestInputModelKeys(t *testing.T) {
	t.Parallel()

	const initialValue = "value"

	tests := []struct {
		name      string
		key       tea.KeyMsg
		wantInput string
		wantValue string
		cancelled bool
		quit      bool
	}{
		{
			name: "enter submits", key: tea.KeyMsg{Type: tea.KeyEnter},
			wantInput: initialValue, wantValue: initialValue, quit: true,
		},
		{
			name: "escape cancels", key: tea.KeyMsg{Type: tea.KeyEsc},
			wantInput: initialValue, cancelled: true, quit: true,
		},
		{
			name: "ctrl c cancels", key: tea.KeyMsg{Type: tea.KeyCtrlC},
			wantInput: initialValue, cancelled: true, quit: true,
		},
		{
			name: "runes reach input", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}},
			wantInput: "valuex",
		},
		{
			name: "backspace reaches input", key: tea.KeyMsg{Type: tea.KeyBackspace},
			wantInput: "valu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ti := textinput.New()
			ti.SetValue(initialValue)
			ti.Focus()
			m := inputModel{textInput: ti}
			updated, cmd := m.Update(tt.key)
			m, ok := updated.(inputModel)
			if !ok {
				t.Fatalf("unexpected input model type %T", updated)
			}
			if m.textInput.Value() != tt.wantInput || m.value != tt.wantValue || m.cancelled != tt.cancelled {
				t.Errorf("Update() input=%q value=%q cancelled=%v, want %q %q %v",
					m.textInput.Value(), m.value, m.cancelled, tt.wantInput, tt.wantValue, tt.cancelled)
			}
			if tt.quit {
				if cmd == nil {
					t.Fatal("expected quit command")
				}
				if msg := cmd(); msg != tea.Quit() {
					t.Errorf("command returned %T, want tea.QuitMsg", msg)
				}
			}
		})
	}
}
