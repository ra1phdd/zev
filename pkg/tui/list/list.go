package list

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SelectOptions конфигурация для выбора.
type SelectOptions struct {
	Title       string
	Items       []Item
	ShowWarning bool // показывать предупреждения для опасных элементов
}

// Item представляет элемент списка.
type Item interface {
	Title() string
	Description() string
	IsWarning() bool // для опасных команд
}

// SimpleItem простой текстовый элемент.
type SimpleItem struct {
	title       string
	description string
}

func (i SimpleItem) Title() string       { return i.title }
func (i SimpleItem) Description() string { return i.description }
func (i SimpleItem) IsWarning() bool     { return false }

// NewSimpleItem создаёт простой элемент списка.
func NewSimpleItem(title, description string) SimpleItem {
	return SimpleItem{title: title, description: description}
}

// Select показывает интерактивный выбор из списка.
func Select(opts SelectOptions) (Item, error) {
	if len(opts.Items) == 0 {
		return nil, errors.New("no items to select from")
	}

	m := newModel(opts)
	p := tea.NewProgram(m)

	result, err := p.Run()
	if err != nil {
		return nil, err
	}

	finalModel := result.(model)
	if finalModel.cancelled {
		return nil, errors.New("cancelled")
	}

	return finalModel.selected, nil
}

type model struct {
	title       string
	items       []Item
	showWarning bool
	cursor      int
	selected    Item
	cancelled   bool
}

func newModel(opts SelectOptions) model {
	return model{
		title:       opts.Title,
		items:       opts.Items,
		showWarning: opts.ShowWarning,
		cursor:      0,
	}
}

var (
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	regularStyle = lipgloss.NewStyle()
)

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp:
		m.cursor--
		if m.cursor < 0 {
			m.cursor = len(m.items) - 1
		}

	case tea.KeyDown:
		m.cursor++
		if m.cursor >= len(m.items) {
			m.cursor = 0
		}

	case tea.KeyEnter:
		m.selected = m.items[m.cursor]
		return m, tea.Quit

	case tea.KeyCtrlC, tea.KeyEsc:
		m.cancelled = true
		return m, tea.Quit
	}

	// Handle 'q' key separately (it comes as KeyRunes)
	if keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'q' {
		m.cancelled = true
		return m, tea.Quit
	}

	return m, nil
}

func (m model) View() string {
	var b strings.Builder

	if m.title != "" {
		b.WriteString(m.title)
		b.WriteString("\n\n")
	}

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		prefix := ""
		if m.showWarning && item.IsWarning() {
			prefix = "⚠️  "
		}

		line := fmt.Sprintf("%s%s%d. %s", cursor, prefix, i+1, item.Title())

		if i == m.cursor {
			line = cursorStyle.Render(line)
		} else {
			line = regularStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")

		if item.Description() != "" {
			desc := "   " + item.Description()
			b.WriteString(desc)
			b.WriteString("\n")
		}
	}

	return b.String()
}
