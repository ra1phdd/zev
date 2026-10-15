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
	delegate.ShowDescription = false
	delegate.SetHeight(1)   // одна строка на item
	delegate.SetSpacing(0)  // без отступов

	// Компактный размер
	l := list.New(items, delegate, 60, len(items)+3)
	l.Title = promptText
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false) // убираем help

	m := selectModel{list: l}
	p := tea.NewProgram(m) // убираем tea.WithAltScreen() для inline режима

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
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
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
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m selectModel) View() string {
	return m.list.View()
}
