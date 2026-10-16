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
	width     int // ширина терминала
}

func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

// wrapText переносит текст на несколько строк с учётом максимальной ширины.
func wrapText(text string, maxWidth int) string {
	if len(text) <= maxWidth {
		return text
	}

	var lines []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxWidth {
			lines = append(lines, remaining)
			break
		}

		// Разрыв по последнему пробелу
		breakPoint := strings.LastIndex(remaining[:maxWidth], " ")
		if breakPoint == -1 {
			// Нет пробелов — режем по ширине
			breakPoint = maxWidth
			lines = append(lines, remaining[:breakPoint])
			remaining = remaining[breakPoint:]
		} else {
			// Есть пробел — режем по нему
			lines = append(lines, remaining[:breakPoint])
			remaining = strings.TrimSpace(remaining[breakPoint:])
		}
	}

	return strings.Join(lines, "\n")
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.textInput.Width = msg.Width - 4 // учитываем промежуток
		return m, nil

	case tea.KeyMsg:
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
	maxWidth := m.width
	if maxWidth == 0 {
		maxWidth = 80
	}

	wrappedPrompt := wrapText(m.prompt, maxWidth)

	return fmt.Sprintf("%s\n\n%s\n\n(Press Enter to confirm, Esc to cancel)",
		wrappedPrompt,
		m.textInput.View(),
	)
}
