// Package prompt provides interactive terminal text input.
package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ra1phdd/zev/locale"
)

// InputContext displays promptText and returns trimmed input. Cancellation is
// reported with an error matching ctx.Err() through errors.Is.
func InputContext(ctx context.Context, promptText string) (string, error) {
	return inputContext(ctx, promptText)
}

// SecretInputContext displays promptText and masks typed input. It returns the
// trimmed secret on submission, or an empty value on cancellation or UI failure.
// Errors never format input contents; errors.Is can inspect their underlying cause.
// Callers must not include secrets in promptText.
func SecretInputContext(ctx context.Context, promptText string) (string, error) {
	return secretInputContext(ctx, promptText)
}

type inputError struct{ cause error }

func (e inputError) Error() string { return "input UI failed or canceled" }
func (e inputError) Unwrap() error { return e.cause }

func inputContext(ctx context.Context, promptText string, opts ...tea.ProgramOption) (string, error) {
	return runInputContext(ctx, newInputModel(promptText, false), opts...)
}

func secretInputContext(ctx context.Context, promptText string, opts ...tea.ProgramOption) (string, error) {
	return runInputContext(ctx, newInputModel(promptText, true), opts...)
}

func newInputModel(promptText string, secret bool) inputModel {
	ti := textinput.New()
	if secret {
		ti.EchoMode = textinput.EchoPassword
	}
	ti.Focus()
	return inputModel{textInput: ti, prompt: promptText}
}

func runInputContext(ctx context.Context, m inputModel, opts ...tea.ProgramOption) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", inputError{cause: err}
	}
	p := tea.NewProgram(m, append(opts, tea.WithContext(ctx))...)
	result, err := p.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", inputError{cause: ctxErr}
	}
	if err != nil {
		return "", inputError{cause: err}
	}

	finalModel, ok := result.(inputModel)
	if !ok {
		return "", fmt.Errorf("unexpected input model type %T", result)
	}
	if finalModel.cancelled {
		return "", errors.New("cancelled")
	}

	return strings.TrimSpace(finalModel.value), nil
}

type inputModel struct {
	textInput textinput.Model
	prompt    string
	value     string
	cancelled bool
	width     int // Terminal width.
}

func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

// wrapText wraps without splitting UTF-8 or stalling on a narrow terminal.
func wrapText(text string, maxWidth int) string {
	maxWidth = max(1, maxWidth)
	remaining := []rune(text)
	if len(remaining) <= maxWidth {
		return text
	}
	var lines []string
	for len(remaining) > 0 {
		if len(remaining) <= maxWidth {
			lines = append(lines, string(remaining))
			break
		}
		breakPoint := maxWidth
		for i := maxWidth - 1; i > 0; i-- {
			if remaining[i] == ' ' {
				breakPoint = i
				break
			}
		}
		lines = append(lines, string(remaining[:breakPoint]))
		remaining = remaining[breakPoint:]
		if breakPoint < maxWidth {
			remaining = []rune(strings.TrimSpace(string(remaining)))
		}
	}
	return strings.Join(lines, "\n")
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.textInput.Width = max(1, msg.Width-4)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.value = m.textInput.Value()
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		default:
			// Forward editing keys to the text input below.
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m inputModel) View() string {
	maxWidth := m.width
	if maxWidth == 0 {
		maxWidth = 80
	}

	wrappedPrompt := wrapText(m.prompt, maxWidth)

	return fmt.Sprintf("%s\n\n%s\n\n%s",
		wrappedPrompt,
		m.textInput.View(),
		locale.Text("(Press Enter to confirm, Esc to cancel)"),
	)
}
