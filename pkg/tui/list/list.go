// Package list provides interactive selection of terminal commands and text items.
package list

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SelectOptions configures the list title, required items, and optional warnings.
type SelectOptions struct {
	Title       string
	Items       []Item
	ShowWarning bool // Display warnings for dangerous items.
}

// Item provides the display content and warning state of a selectable entry.
type Item interface {
	Title() string
	Description() string
	IsWarning() bool
}

// SimpleItem is a text entry without a danger warning.
type SimpleItem struct {
	title       string
	description string
}

// Title returns the entry's display text.
func (i SimpleItem) Title() string { return i.title }

// Description returns the entry's optional explanation.
func (i SimpleItem) Description() string { return i.description }

// IsWarning returns false because simple text entries have no danger flag.
func (i SimpleItem) IsWarning() bool { return false }

// NewSimpleItem creates an entry with title and an optional description.
func NewSimpleItem(title, description string) SimpleItem {
	return SimpleItem{title: title, description: description}
}

// Select returns the item chosen interactively from opts.Items.
// It returns an error for an empty list, cancellation, or a terminal UI failure.
func Select(opts SelectOptions) (Item, error) {
	if len(opts.Items) == 0 {
		return nil, errors.New("no items to select from")
	}

	m := newModel(opts)
	p := tea.NewProgram(m)

	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run selection UI: %w", err)
	}

	finalModel, ok := result.(model)
	if !ok {
		return nil, fmt.Errorf("unexpected selection model type %T", result)
	}
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
	width       int // ширина терминала
}

func newModel(opts SelectOptions) model {
	return model{
		title:       opts.Title,
		items:       opts.Items,
		showWarning: opts.ShowWarning,
		cursor:      0,
		width:       80, // дефолтная ширина
	}
}

var (
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	regularStyle = lipgloss.NewStyle()
)

// wrapText переносит текст на несколько строк с учётом максимальной ширины.
// indent — отступ для всех строк кроме первой.
func wrapText(text string, maxWidth, indent int) string {
	if len(text) <= maxWidth {
		return text
	}

	var lines []string
	remaining := text
	firstLine := true

	for len(remaining) > 0 {
		lineWidth := maxWidth
		if !firstLine {
			lineWidth = maxWidth - indent
		}

		if len(remaining) <= lineWidth {
			if firstLine {
				lines = append(lines, remaining)
			} else {
				lines = append(lines, strings.Repeat(" ", indent)+remaining)
			}
			break
		}

		breakPoint := strings.LastIndex(remaining[:lineWidth], " ")
		hardBreak := breakPoint == -1
		if hardBreak {
			breakPoint = lineWidth
		}
		line := remaining[:breakPoint]
		if !firstLine {
			line = strings.Repeat(" ", indent) + line
		}
		lines = append(lines, line)
		firstLine = false
		remaining = remaining[breakPoint:]
		if !hardBreak {
			remaining = strings.TrimSpace(remaining)
		}
	}

	return strings.Join(lines, "\n")
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
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
		default:
			// Other keys do not navigate or select; rune shortcuts are handled below.
		}

		// Handle 'q' key separately (it comes as KeyRunes)
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
			m.cancelled = true
			return m, tea.Quit
		}
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

		// Вычисляем длину префиксов для правильного переноса
		// cursor (2) + prefix (0 или 4) + "N. " (примерно 3)
		prefixLen := len(cursor) + len(prefix) + len(fmt.Sprintf("%d. ", i+1))

		// Оборачиваем Title с учётом префиксов
		titleText := wrapText(item.Title(), m.width-prefixLen, prefixLen)

		line := fmt.Sprintf("%s%s%d. %s", cursor, prefix, i+1, titleText)

		if i == m.cursor {
			line = cursorStyle.Render(line)
		} else {
			line = regularStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")

		if item.Description() != "" {
			// Оборачиваем Description с отступом 3
			descText := wrapText(item.Description(), m.width-3, 3)
			desc := "   " + descText
			b.WriteString(desc)
			b.WriteString("\n")
		}
	}

	return b.String()
}
