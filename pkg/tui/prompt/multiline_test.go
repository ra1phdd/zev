package prompt

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func requireMultilineModel(t *testing.T, model tea.Model) multilineModel {
	t.Helper()
	m, ok := model.(multilineModel)
	if !ok {
		t.Fatalf("unexpected multiline model type %T", model)
	}
	return m
}

func TestMultilineTerminalStateIsFinal(t *testing.T) {
	t.Parallel()

	for _, key := range []tea.KeyType{tea.KeyCtrlD, tea.KeyEsc, tea.KeyCtrlC} {
		m := newMultilineModel("Log")
		updated, _ := m.Update(tea.KeyMsg{Type: key})
		m = requireMultilineModel(t, updated)
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("late input")})
		m = requireMultilineModel(t, updated)
		if m.textArea.Value() != "" {
			t.Fatalf("late input changed terminal model after %s", tea.KeyMsg{Type: key}.String())
		}
	}
	m := newMultilineModel("Log")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("x", 64*1024+1)), Paste: true})
	m = requireMultilineModel(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = requireMultilineModel(t, updated)
	if m.err == nil || m.submitted {
		t.Fatal("late submit overwrote input rejection")
	}
}

func TestMultilineModelResizesViewport(t *testing.T) {
	t.Parallel()

	m := newMultilineModel("Log")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = requireMultilineModel(t, updated)
	if m.textArea.Width() > 40 || m.textArea.Height() > 4 {
		t.Fatalf("viewport = %dx%d, does not fit terminal", m.textArea.Width(), m.textArea.Height())
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	m = requireMultilineModel(t, updated)
	if m.textArea.Width() < 1 || m.textArea.Height() < 1 {
		t.Fatal("viewport has nonpositive dimensions")
	}
	view := m.View()
	for _, hint := range []string{"Log", "Enter", "Ctrl+D", "Esc", "Ctrl+C", "64 KiB"} {
		if !strings.Contains(view, hint) {
			t.Errorf("view lacks %q", hint)
		}
	}
}

func TestMultilineClipboardMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		err  error
		fail bool
	}{
		{"preserved lines", " first\n\nlast\n", nil, false},
		{"oversized bytes", strings.Repeat("x", 64*1024+1), nil, true},
		{"oversized lines", strings.Repeat("\n", 10000), nil, true},
		{"clipboard failure", "", errors.New("private clipboard error"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newMultilineModel("Log")
			updated, cmd := m.Update(multilinePasteMsg{text: tt.text, err: tt.err})
			m = requireMultilineModel(t, updated)
			if (m.err != nil) != tt.fail {
				t.Fatalf("clipboard rejection = %v; want failure %v", m.err, tt.fail)
			}
			if tt.fail {
				if m.textArea.Value() != "" || cmd == nil || cmd() != tea.Quit() {
					t.Fatal("clipboard rejection changed text or did not quit")
				}
				if strings.Contains(m.err.Error(), "private clipboard error") {
					t.Fatal("clipboard failure exposed contents")
				}
			} else if m.textArea.Value() != tt.text {
				t.Fatal("clipboard text was not preserved")
			}
		})
	}
	m := newMultilineModel("Log")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	if cmd == nil {
		t.Fatal("Ctrl+V did not request clipboard input")
	}
}

func TestMultilineModelRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	const limit = 64 * 1024
	tests := []struct {
		name    string
		initial string
		msg     tea.KeyMsg
	}{
		{"oversized paste", "", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("private-log" + strings.Repeat("x", limit)), Paste: true}},
		{"UTF-8 bytes", "", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("я", limit/2+1)), Paste: true}},
		{"cumulative paste", strings.Repeat("x", limit-1), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab"), Paste: true}},
		{"newline at limit", strings.Repeat("x", limit), tea.KeyMsg{Type: tea.KeyEnter}},
		{"expanded tabs", "", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("\t", limit/4+1)), Paste: true}},
		{"textarea line limit", "", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("\n", 10000)), Paste: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newMultilineModel("Log")
			m.textArea.SetValue(tt.initial)
			updated, cmd := m.Update(tt.msg)
			m = requireMultilineModel(t, updated)
			if m.err == nil {
				t.Fatal("oversized input was accepted or silently truncated")
			}
			if strings.Contains(m.err.Error(), "private-log") || m.value != "" || m.submitted {
				t.Fatal("rejection exposed or submitted input")
			}
			if m.textArea.Value() != tt.initial {
				t.Fatal("rejected paste changed the editor contents")
			}
			if cmd == nil || cmd() != tea.Quit() {
				t.Fatal("oversized input did not quit with an error")
			}
		})
	}
}

func TestMultilineModelAcceptsExactByteLimit(t *testing.T) {
	t.Parallel()

	for _, text := range []string{strings.Repeat("x", 64*1024), strings.Repeat("я", 32*1024)} {
		m := newMultilineModel("Log")
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
		m = requireMultilineModel(t, updated)
		if m.err != nil || m.textArea.Value() != text {
			t.Fatalf("exact byte limit rejected or truncated: %v", m.err)
		}
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		m = requireMultilineModel(t, updated)
		if m.value != text || !m.submitted {
			t.Fatal("exact byte limit was not submitted intact")
		}
	}
}

func TestMultilineInputContextRejectsOversizedPaste(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	input := "\x1b[200~private-log" + strings.Repeat("x", 64*1024) + "\x1b[201~\x04"
	got, err := runMultilineContext(ctx, newMultilineModel("Log"),
		tea.WithInput(strings.NewReader(input)), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	if got != "" || err == nil {
		t.Fatal("oversized paste returned text or no error")
	}
	if !strings.Contains(err.Error(), "64 KiB") || strings.Contains(err.Error(), "private-log") {
		t.Fatalf("size error = %v; want explicit, contents-free size error", err)
	}
}

func TestMultilineCancellation(t *testing.T) {
	t.Parallel()

	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		t.Run(tea.KeyMsg{Type: key}.String(), func(t *testing.T) {
			t.Parallel()
			m := newMultilineModel("Log")
			m.textArea.SetValue("private contents")
			updated, cmd := m.Update(tea.KeyMsg{Type: key})
			var ok bool
			m, ok = updated.(multilineModel)
			if !ok {
				t.Fatal("unexpected multiline model")
			}
			if !m.cancelled || m.value != "" || m.submitted {
				t.Fatal("cancellation retained a result or submitted")
			}
			if cmd == nil || cmd() != tea.Quit() {
				t.Fatal("cancellation did not quit")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			input := "\x1b"
			if key == tea.KeyCtrlC {
				input = "\x03"
			}
			got, err := runMultilineContext(ctx, newMultilineModel("Log"),
				tea.WithInput(strings.NewReader(input)), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
			if got != "" || err == nil || strings.Contains(err.Error(), "private contents") {
				t.Fatalf("canceled result = %q, %v; want empty value and safe error", got, err)
			}
		})
	}
}

func TestMultilineContextCancelsRunningPrompt(t *testing.T) {
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
		got, err := runMultilineContext(ctx, newMultilineModel("private title"),
			tea.WithInput(reader), tea.WithOutput(readyWriter{ready}), tea.WithoutSignalHandler())
		if got != "" {
			done <- errors.New("cancellation returned text")
			return
		}
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
			t.Fatalf("running prompt error = %v; want context.Canceled", err)
		}
		if strings.Contains(err.Error(), "private title") {
			t.Fatal("error exposed the title")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not stop after cancellation")
	}
}

func TestMultilineInputContextAlreadyCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := MultilineInputContext(ctx, "private title")
	if got != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("MultilineInputContext = %q, %v; want empty value and context.Canceled", got, err)
	}
	if strings.Contains(err.Error(), "private title") {
		t.Fatal("error exposed the title")
	}
}

func TestMultilineInputContextSubmission(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "\n", "\n\n", " \n\nlast \n"} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			input := "\x1b[200~" + text + "\x1b[201~\x04"
			got, err := runMultilineContext(ctx, newMultilineModel("Log"),
				tea.WithInput(strings.NewReader(input)), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
			if err != nil || got != text {
				t.Fatalf("submission = %q, %v; want %q, nil", got, err, text)
			}
		})
	}
}

func TestMultilineModelPreservesLinesAndSubmits(t *testing.T) {
	t.Parallel()

	m := newMultilineModel("Command log")
	messages := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" first ")},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("last ")},
		tea.KeyMsg{Type: tea.KeyEnter},
	}
	for _, msg := range messages {
		updated, _ := m.Update(msg)
		m = requireMultilineModel(t, updated)
		if m.submitted {
			t.Fatal("editing submitted the prompt")
		}
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = requireMultilineModel(t, updated)
	if !m.submitted || m.value != " first \n\nlast \n" {
		t.Fatalf("submission = %q, submitted = %v; want preserved multiline text", m.value, m.submitted)
	}
	if cmd == nil || cmd() != tea.Quit() {
		t.Fatal("Ctrl+D did not quit")
	}
}
