package selector

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ra1phdd/zev-go/internal/core/domain"
)

// Select показывает интерактивный выбор команды из списка.
func Select(commands []domain.Command) (*domain.Command, error) {
	if len(commands) == 0 {
		return nil, errors.New("no commands to select from")
	}

	items := make([]list.Item, len(commands))
	for i := range commands {
		items[i] = commandItem{cmd: &commands[i]}
	}

	delegate := commandDelegate{}
	height := len(items) * 2

	l := list.New(items, delegate, 80, height)
	l.Title = "Select a command to run:"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowTitle(true)
	l.SetShowPagination(false)
	l.SetShowFilter(false)

	l.Styles.Title = lipgloss.NewStyle().Margin(0).Padding(0)
	l.Styles.TitleBar = lipgloss.NewStyle().Margin(0).Padding(0)
	l.Styles.PaginationStyle = lipgloss.NewStyle().Margin(0).Padding(0)
	l.Styles.HelpStyle = lipgloss.NewStyle().Margin(0).Padding(0)

	m := model{list: l}
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
	list      list.Model
	selected  *domain.Command
	cancelled bool
}

type commandItem struct {
	cmd *domain.Command
}

func (i commandItem) Title() string { return i.cmd.Command }
func (i commandItem) Description() string {
	desc := i.cmd.ShortExplanation
	if i.cmd.IsDangerous {
		desc = "⚠️  " + desc + " (DANGEROUS)"
	}
	return desc
}
func (i commandItem) FilterValue() string { return i.cmd.Command }

type commandDelegate struct{}

func (d commandDelegate) Height() int                             { return 1 }
func (d commandDelegate) Spacing() int                            { return 0 }
func (d commandDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d commandDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(commandItem)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i.Title())

	fn := itemStyle.Render
	if index == m.Index() {
		fn = selectedItemStyle.Render
	}

	fmt.Fprint(w, fn(str))
}

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(1)
	selectedItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
)

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			i, ok := m.list.SelectedItem().(commandItem)
			if ok {
				m.selected = i.cmd
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

func (m model) View() string {
	return m.list.View()
}
