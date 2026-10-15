package prompt

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Input показывает текстовый промпт и возвращает введённое значение.
func Input(promptText string) (string, error) {
	ti := textinput.New()
	ti.Placeholder = promptText
	ti.Focus()

	m := inputModel{
		textInput: ti,
		prompt:    promptText,
	}

	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return "", err
	}

	finalModel := result.(inputModel)
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
}

func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.Type {
		case tea.KeyEnter:
			m.value = m.textInput.Value()
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m inputModel) View() string {
	return fmt.Sprintf("%s\n\n%s\n\n(Press Enter to confirm, Esc to cancel)",
		m.prompt,
		m.textInput.View(),
	)
}
