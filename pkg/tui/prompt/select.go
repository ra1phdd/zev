package prompt

import (
	"errors"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Select показывает список опций для выбора.
func Select(promptText string, options []string) (string, error) {
	items := make([]list.Item, len(options))
	for i, opt := range options {
		items[i] = item(opt)
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = promptText
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	m := selectModel{list: l}
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		return "", err
	}

	finalModel := result.(selectModel)
	if finalModel.cancelled {
		return "", errors.New("cancelled")
	}

	return finalModel.choice, nil
}

type selectModel struct {
	list      list.Model
	choice    string
	cancelled bool
}

type item string

func (i item) Title() string       { return string(i) }
func (i item) Description() string { return "" }
func (i item) FilterValue() string { return string(i) }

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			i, ok := m.list.SelectedItem().(item)
			if ok {
				m.choice = string(i)
			}
			return m, tea.Quit
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		h, v := msg.Width, msg.Height
		m.list.SetSize(h, v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m selectModel) View() string {
	return m.list.View()
}
