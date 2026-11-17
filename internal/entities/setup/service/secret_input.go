package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ra1phdd/zev/locale"
)

// secretInputError preserves error identity without formatting UI error text,
// which can include input values.
type secretInputError struct {
	cause error
}

func (e secretInputError) Error() string {
	return "read API key: input UI failed"
}

func (e secretInputError) Unwrap() error {
	return e.cause
}

func (s *Service) promptAPIKey(ctx context.Context, provider, current string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", secretInputError{cause: err}
	}
	promptText := fmt.Sprintf(locale.Text(locale.KeyUISetupAPIKey), provider)
	if current != "" {
		promptText += fmt.Sprintf(locale.Text(locale.KeyUISetupCurrentAPIKey), maskAPIKey(current))
	}
	m := newAPIKeyInput(promptText + ":")
	var result tea.Model
	var err error
	if s.runSecretInput != nil {
		result, err = s.runSecretInput(m)
	} else {
		result, err = tea.NewProgram(m, tea.WithContext(ctx)).Run()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", secretInputError{cause: ctxErr}
	}
	if err != nil {
		return "", secretInputError{cause: err}
	}
	final, ok := result.(apiKeyInput)
	if !ok {
		return "", errors.New("read API key: unexpected input model")
	}
	if final.cancelled {
		return "", errors.New("read API key: cancelled")
	}
	value := strings.TrimSpace(final.textInput.Value())
	if value == "" {
		return current, nil
	}
	return value, nil
}

type apiKeyInput struct {
	textInput textinput.Model
	prompt    string
	cancelled bool
}

func newAPIKeyInput(promptText string) apiKeyInput {
	ti := textinput.New()
	ti.EchoMode = textinput.EchoPassword
	ti.Focus()
	return apiKeyInput{textInput: ti, prompt: promptText}
}

func (m apiKeyInput) Init() tea.Cmd {
	return textinput.Blink
}

func (m apiKeyInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.textInput.Width = max(1, msg.Width-4)
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			return m, tea.Quit
		case tea.KeyEsc, tea.KeyCtrlC:
			m.cancelled = true
			m.textInput.Reset()
			return m, tea.Quit
		default:
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m apiKeyInput) View() string {
	return fmt.Sprintf("%s\n\n%s\n\n%s", m.prompt, m.textInput.View(), locale.Text(locale.KeyUIConfirmHint))
}
