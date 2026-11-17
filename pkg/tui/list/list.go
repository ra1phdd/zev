// Package list provides interactive selection of terminal commands and text items.
package list

import (
	"context"
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

// SelectContext returns the chosen item, or an error for empty items, terminal
// failure, or cancellation. Context errors are preserved for errors.Is.
func SelectContext(ctx context.Context, opts SelectOptions) (Item, error) {
	return selectContext(ctx, opts)
}

func selectContext(ctx context.Context, opts SelectOptions, programOpts ...tea.ProgramOption) (Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("selection canceled: %w", err)
	}
	if len(opts.Items) == 0 {
		return nil, errors.New("no items to select from")
	}

	m := newModel(opts)
	p := tea.NewProgram(m, append(programOpts, tea.WithContext(ctx))...)

	result, err := p.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("selection canceled: %w", ctxErr)
	}
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

// wrapText keeps UTF-8 intact and always advances, even when the terminal is
// narrower than the selection prefix or continuation indentation.
func wrapText(text string, maxWidth, indent int) string {
	maxWidth = max(1, maxWidth)
	indent = max(0, min(indent, maxWidth-1))
	remaining := []rune(text)
	if len(remaining) <= maxWidth {
		return text
	}
	var lines []string
	firstLine := true
	for len(remaining) > 0 {
		lineWidth := maxWidth
		padding := ""
		if !firstLine {
			lineWidth -= indent
			padding = strings.Repeat(" ", indent)
		}
		if len(remaining) <= lineWidth {
			lines = append(lines, padding+string(remaining))
			break
		}
		breakPoint := lineWidth
		for i := lineWidth - 1; i > 0; i-- {
			if remaining[i] == ' ' {
				breakPoint = i
				break
			}
		}
		lines = append(lines, padding+string(remaining[:breakPoint]))
		remaining = remaining[breakPoint:]
		if breakPoint < lineWidth {
			remaining = []rune(strings.TrimSpace(string(remaining)))
		}
		firstLine = false
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
