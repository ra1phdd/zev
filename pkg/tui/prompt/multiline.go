package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/runeutil"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ra1phdd/zev/internal/core/locale"
)

const (
	multilineMaxBytes = 64 * 1024
	// Bubbles v1.0.0 silently truncates pasted text above this hard-coded limit.
	multilineMaxLines = 10000
)

// MultilineInputContext displays title and returns submitted text without trimming.
// Enter inserts a newline; Ctrl+D submits, including an empty value to skip.
// Esc and Ctrl+C return a cancellation error. Context errors match ctx.Err() with
// errors.Is. UI errors never include input contents. Input is limited to 64 KiB
// and the textarea's 10000-line limit; exceeding either returns an explicit error.
func MultilineInputContext(ctx context.Context, title string) (string, error) {
	return runMultilineContextWithInput(ctx, newMultilineModel(title), os.Stdin)
}

// MultilineInputContextWithInput displays title using input as the terminal reader.
func MultilineInputContextWithInput(ctx context.Context, title string, input io.Reader) (string, error) {
	return runMultilineContextWithInput(ctx, newMultilineModel(title), input)
}

func runMultilineContext(ctx context.Context, m multilineModel, opts ...tea.ProgramOption) (string, error) {
	return runMultilineProgram(ctx, m, opts...)
}

func runMultilineContextWithInput(ctx context.Context, m multilineModel, input io.Reader) (string, error) {
	if input == nil {
		input = os.Stdin
	}
	return runMultilineProgram(ctx, m, tea.WithInput(&cancelOnEOFReader{Reader: input}))
}

func runMultilineProgram(ctx context.Context, m multilineModel, opts ...tea.ProgramOption) (string, error) {
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
	finalModel, ok := result.(multilineModel)
	if !ok {
		return "", fmt.Errorf("unexpected multiline input model type %T", result)
	}
	if finalModel.cancelled {
		return "", errors.New("cancelled")
	}
	if finalModel.err != nil {
		return "", finalModel.err
	}
	return finalModel.value, nil
}

type multilinePasteMsg struct {
	text string
	err  error
}

type multilineModel struct {
	textArea  textarea.Model
	title     string
	value     string
	submitted bool
	cancelled bool
	err       error
}

func newMultilineModel(title string) multilineModel {
	ta := textarea.New()
	ta.MaxHeight = 0
	ta.SetWidth(76)
	ta.SetHeight(6)
	ta.Focus()
	return multilineModel{textArea: ta, title: title}
}

func (m multilineModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m multilineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.submitted || m.cancelled || m.err != nil {
		return m, tea.Quit
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.textArea.SetWidth(max(1, size.Width-4))
		m.textArea.SetHeight(max(1, min(6, size.Height-6)))
		return m, nil
	}
	if paste, ok := msg.(multilinePasteMsg); ok {
		if paste.err != nil {
			m.err = inputError{cause: paste.err}
			return m, tea.Quit
		}
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(paste.text), Paste: true}
	}
	var insertion string
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyCtrlV:
			// Route clipboard text through the same checks as terminal paste.
			return m, func() tea.Msg {
				text, err := clipboard.ReadAll()
				return multilinePasteMsg{text: text, err: err}
			}
		case tea.KeyCtrlD:
			m.value = m.textArea.Value()
			m.submitted = true
			return m, tea.Quit
		case tea.KeyEsc, tea.KeyCtrlC:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyRunes:
			if !key.Alt {
				if len(string(key.Runes)) > multilineMaxBytes {
					m.err = errors.New("multiline input exceeds 64 KiB")
					return m, tea.Quit
				}
				key.Runes = runeutil.NewSanitizer().Sanitize(slices.Clone(key.Runes))
				insertion = string(key.Runes)
				msg = key
			}
		case tea.KeyEnter:
			insertion = "\n"
		default:
			// Let textarea handle navigation and editing keys.
		}
	}
	if len(m.textArea.Value())+len(insertion) > multilineMaxBytes {
		m.err = errors.New("multiline input exceeds 64 KiB")
		return m, tea.Quit
	}
	if m.textArea.LineCount()+strings.Count(insertion, "\n") > multilineMaxLines {
		m.err = errors.New("multiline input exceeds 10000 lines")
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	if len(m.textArea.Value()) > multilineMaxBytes {
		m.err = errors.New("multiline input exceeds 64 KiB")
		return m, tea.Quit
	}
	return m, cmd
}

func (m multilineModel) View() string {
	return fmt.Sprintf("%s\n\n%s\n\n%s", m.title, m.textArea.View(), locale.Text(locale.KeyUIMultilineHint))
}
