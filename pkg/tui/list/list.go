// Package list provides interactive selection of terminal commands and text items.
package list

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ra1phdd/zev/internal/core/locale"
)

// SelectOptions configures the list title, required items, and optional warnings.
type SelectOptions struct {
	Title       string
	Items       []Item
	ShowWarning bool // Display warnings for dangerous items.
	Input       io.Reader
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
	inputCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	input := opts.Input
	if input == nil {
		input = os.Stdin
	}
	options := make([]tea.ProgramOption, 0, len(programOpts)+2)
	options = append(options, tea.WithContext(inputCtx), tea.WithInput(&cancelOnEOFReader{Reader: input}))
	options = append(options, programOpts...)
	p := tea.NewProgram(m, options...)

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

type cancelOnEOFReader struct {
	io.Reader
	eofSent  bool
	hadInput bool
}

func (r *cancelOnEOFReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.hadInput = true
	}
	if err == nil {
		return n, nil
	}
	if errors.Is(err, io.EOF) && !r.eofSent && !r.hadInput && len(p) > 0 {
		r.eofSent = true
		p[0] = 3
		return 1, nil
	}
	return n, fmt.Errorf("read input: %w", err)
}

type model struct {
	title        string
	items        []Item
	showWarning  bool
	cursor       int
	selected     Item
	cancelled    bool
	width        int // ширина терминала
	height       int
	firstVisible int
}

func newModel(opts SelectOptions) model {
	return model{
		title:       opts.Title,
		items:       opts.Items,
		showWarning: opts.ShowWarning,
		cursor:      0,
		width:       80, // дефолтная ширина
		height:      24,
	}
}

var (
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	regularStyle = lipgloss.NewStyle()
)

// wrapText keeps UTF-8 intact, respects terminal cells, and always advances.
func wrapText(text string, maxWidth, indent int) string {
	maxWidth = max(1, maxWidth)
	indent = max(0, min(indent, maxWidth-1))
	parts := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(parts))
	for i, part := range parts {
		line := wrapLine(part, maxWidth, indent)
		if i > 0 {
			line = strings.ReplaceAll(line, "\n", "\n"+strings.Repeat(" ", indent))
			line = strings.Repeat(" ", indent) + line
		}
		wrapped = append(wrapped, line)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(text string, maxWidth, indent int) string {
	remaining := []rune(text)
	if ansi.StringWidth(text) <= maxWidth {
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
		fit := fittingRunes(remaining, lineWidth)
		if fit == 0 {
			fit = 1
		}
		if fit == len(remaining) {
			lines = append(lines, padding+string(remaining))
			break
		}
		breakPoint := fit
		for i := fit - 1; i > 0; i-- {
			if remaining[i] == ' ' {
				breakPoint = i
				break
			}
		}
		lines = append(lines, padding+string(remaining[:breakPoint]))
		remaining = remaining[breakPoint:]
		if breakPoint < fit {
			remaining = []rune(strings.TrimSpace(string(remaining)))
		}
		firstLine = false
	}
	return strings.Join(lines, "\n")
}

func fittingRunes(text []rune, maxWidth int) int {
	width := 0
	for i, r := range text {
		runeWidth := ansi.StringWidth(string(r))
		if i > 0 && width+runeWidth > maxWidth {
			return i
		}
		width += runeWidth
		if width >= maxWidth {
			return i + 1
		}
	}
	return len(text)
}

func truncateCells(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	width := 0
	var b strings.Builder
	for _, r := range text {
		runeWidth := ansi.StringWidth(string(r))
		if width+runeWidth > maxWidth {
			break
		}
		b.WriteRune(r)
		width += runeWidth
	}
	return b.String()
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.items) - 1
			}
			m.ensureCursorVisible()

		case tea.KeyDown:
			m.cursor++
			if m.cursor >= len(m.items) {
				m.cursor = 0
			}
			m.ensureCursorVisible()

		case tea.KeyEnter:
			width := m.width
			if width == 0 {
				width = 80
			}
			start, end := m.visibleRange(width)
			if m.cursor < start || m.cursor >= end {
				return m, nil
			}
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
	width := m.width
	if width == 0 {
		width = 80
	}
	start, end := m.visibleRange(width)
	header := []string{}
	if m.title != "" {
		header = []string{m.title, ""}
	}
	hint := strings.Split(m.navigationHint(width), "\n")
	height := max(1, m.height)
	if len(header) > height {
		header = header[:height]
	}
	remainingHeight := max(0, height-len(header))
	hintLines := min(len(hint), remainingHeight)
	remaining := max(0, remainingHeight-hintLines)
	body := make([]string, 0, remaining)
	for i := start; i < end && remaining > 0; i++ {
		item := m.items[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		prefix := ""
		if m.showWarning && item.IsWarning() {
			prefix = "⚠️  "
		}
		prefixLen := ansi.StringWidth(cursor) + ansi.StringWidth(prefix) + ansi.StringWidth(fmt.Sprintf("%d. ", i+1))
		titleText := wrapText(item.Title(), width-prefixLen, prefixLen)
		line := fmt.Sprintf("%s%s%d. %s", cursor, prefix, i+1, titleText)
		if i == m.cursor {
			line = cursorStyle.Render(line)
		} else {
			line = regularStyle.Render(line)
		}
		itemLines := strings.Split(strings.TrimSuffix(line, "\n"), "\n")
		if item.Description() != "" {
			descText := wrapText(item.Description(), width-3, 3)
			itemLines = append(itemLines, strings.Split("   "+descText, "\n")...)
		}
		for lineIndex := range itemLines {
			itemLines[lineIndex] = truncateCells(itemLines[lineIndex], width)
		}
		if len(itemLines) > remaining {
			itemLines = itemLines[:remaining]
		}
		body = append(body, itemLines...)
		remaining -= len(itemLines)
	}

	var b strings.Builder
	lines := append(append([]string{}, header...), body...)
	lines = append(lines, hint[:hintLines]...)
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteByte('\n')
	return b.String()
}

func (m model) navigationHint(width int) string {
	return wrapText(locale.Text(locale.KeyUIListNavigationHint), max(1, width), 0)
}

func (m model) availableHeight(width int) int {
	headerLines := 0
	if m.title != "" {
		headerLines = 2
	}
	hintLines := strings.Count(m.navigationHint(width), "\n") + 1
	return max(0, m.height-headerLines-hintLines)
}

func (m model) itemLines(index, width int) int {
	item := m.items[index]
	cursor := "  "
	if index == m.cursor {
		cursor = "> "
	}
	prefix := ""
	if m.showWarning && item.IsWarning() {
		prefix = "⚠️  "
	}
	prefixLen := ansi.StringWidth(cursor) + ansi.StringWidth(prefix) + ansi.StringWidth(fmt.Sprintf("%d. ", index+1))
	lines := strings.Count(wrapText(item.Title(), width-prefixLen, prefixLen), "\n") + 1
	if item.Description() != "" {
		lines += strings.Count(wrapText(item.Description(), width-3, 3), "\n") + 1
	}
	return lines
}

func (m model) visibleRange(width int) (int, int) {
	if len(m.items) == 0 {
		return 0, 0
	}
	available := m.availableHeight(width)
	if available <= 0 {
		return 0, 0
	}
	start := min(max(m.firstVisible, 0), len(m.items)-1)
	used := 0
	end := start
	for end < len(m.items) {
		lines := m.itemLines(end, width)
		if end > start && used+lines > available {
			break
		}
		used += lines
		end++
		if used >= available {
			break
		}
	}
	if m.cursor < start || m.cursor >= end {
		start = m.cursor
		end = start
		used = 0
		for end < len(m.items) {
			lines := m.itemLines(end, width)
			if end > start && used+lines > available {
				break
			}
			used += lines
			end++
			if used >= available {
				break
			}
		}
	}
	return start, end
}

func (m *model) ensureCursorVisible() {
	if len(m.items) == 0 {
		m.firstVisible = 0
		return
	}
	width := m.width
	if width == 0 {
		width = 80
	}
	start, end := m.visibleRange(width)
	if m.cursor < start || m.cursor >= end {
		m.firstVisible = m.cursor
	}
}
