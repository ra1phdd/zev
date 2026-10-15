package list

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
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
func (i SimpleItem) FilterValue() string { return i.title }

// NewSimpleItem создаёт простой элемент списка.
func NewSimpleItem(title, description string) SimpleItem {
	return SimpleItem{title: title, description: description}
}

// Select показывает интерактивный выбор из списка.
func Select(opts SelectOptions) (Item, error) {
	if len(opts.Items) == 0 {
		return nil, errors.New("no items to select from")
	}

	listItems := make([]list.Item, len(opts.Items))
	for i, item := range opts.Items {
		listItems[i] = wrappedItem{Item: item}
	}

	delegate := itemDelegate{showWarning: opts.ShowWarning}
	height := len(listItems) * 2

	l := list.New(listItems, delegate, 80, height)
	l.Title = opts.Title
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
	selected  Item
	cancelled bool
}

type wrappedItem struct {
	Item
}

func (i wrappedItem) FilterValue() string { return i.Title() }

type itemDelegate struct {
	showWarning bool
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	i, ok := li.(wrappedItem)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i.Title())

	fn := itemStyle.Render
	if index == m.Index() {
		fn = selectedItemStyle.Render
	}

	// Добавляем предупреждение для опасных элементов
	if d.showWarning && i.IsWarning() {
		str = "⚠️  " + str
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
			i, ok := m.list.SelectedItem().(wrappedItem)
			if ok {
				m.selected = i.Item
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
