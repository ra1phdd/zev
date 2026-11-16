package prompt

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestInputContextAlreadyCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := InputContext(ctx, "question"); !errors.Is(err, context.Canceled) {
		t.Fatalf("InputContext error = %v, want context.Canceled", err)
	}
}

type readyWriter struct{ ready chan struct{} }

func (w readyWriter) Write(p []byte) (int, error) {
	select {
	case w.ready <- struct{}{}:
	default:
	}
	return len(p), nil
}

func TestInputContextCancelsRunningPrompt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		cancel()
		if err := reader.Close(); err != nil {
			t.Errorf("close reader: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := inputContext(ctx, "question", tea.WithInput(reader), tea.WithOutput(readyWriter{ready}))
		done <- err
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("inputContext error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not stop after cancellation")
	}
}

func TestSecretInputContextMasksValue(t *testing.T) {
	t.Parallel()

	const secret = "private-credential"
	ctx := t.Context()
	var output strings.Builder
	got, err := secretInputContext(ctx, "Secret:", tea.WithInput(strings.NewReader(secret+"\r")), tea.WithOutput(&output), tea.WithoutSignalHandler())
	if err != nil {
		t.Fatalf("secretInputContext() error = %v", err)
	}
	if got != secret {
		t.Fatalf("secretInputContext() = %q, want %q", got, secret)
	}
	if strings.Contains(output.String(), secret) {
		t.Fatal("secret appeared in terminal output")
	}
}

func TestSecretInputContextCancellationDoesNotExposeValue(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := SecretInputContext(ctx, "Secret:")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SecretInputContext() error = %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), "Secret:") {
		t.Fatalf("error exposed prompt text: %v", err)
	}
}

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
