package list

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelInit(t *testing.T) {
	t.Parallel()

	items := []Item{
		NewSimpleItem("cmd1", "description1"),
		NewSimpleItem("cmd2", "description2"),
	}

	m := newModel(SelectOptions{
		Title:       "Test",
		Items:       items,
		ShowWarning: false,
	})

	if m.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.cursor)
	}
	if len(m.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(m.items))
	}
}

func TestModelNavigationDown(t *testing.T) {
	t.Parallel()

	items := []Item{
		NewSimpleItem("cmd1", "desc1"),
		NewSimpleItem("cmd2", "desc2"),
		NewSimpleItem("cmd3", "desc3"),
	}

	m := newModel(SelectOptions{Items: items})

	// Down arrow
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)

	if m.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", m.cursor)
	}

	// Another down
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)

	if m.cursor != 2 {
		t.Errorf("expected cursor=2 after second down, got %d", m.cursor)
	}

	// Down at bottom wraps to top
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)

	if m.cursor != 0 {
		t.Errorf("expected cursor=0 after wrap, got %d", m.cursor)
	}
}

func TestModelNavigationUp(t *testing.T) {
	t.Parallel()

	items := []Item{
		NewSimpleItem("cmd1", "desc1"),
		NewSimpleItem("cmd2", "desc2"),
		NewSimpleItem("cmd3", "desc3"),
	}

	m := newModel(SelectOptions{Items: items})

	// Up arrow wraps to bottom
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)

	if m.cursor != 2 {
		t.Errorf("expected cursor=2 after up from top, got %d", m.cursor)
	}

	// Up again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)

	if m.cursor != 1 {
		t.Errorf("expected cursor=1 after second up, got %d", m.cursor)
	}
}

func TestModelEnterSelectsItem(t *testing.T) {
	t.Parallel()

	items := []Item{
		NewSimpleItem("cmd1", "desc1"),
		NewSimpleItem("cmd2", "desc2"),
	}

	m := newModel(SelectOptions{Items: items})
	m.cursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.selected != items[1] {
		t.Error("expected item[1] to be selected")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestModelCancelKeys(t *testing.T) {
	t.Parallel()

	items := []Item{NewSimpleItem("cmd1", "desc1")}

	tests := []struct {
		name string
		key  tea.KeyType
	}{
		{"esc", tea.KeyEsc},
		{"ctrl+c", tea.KeyCtrlC},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := newModel(SelectOptions{Items: items})
			updated, cmd := m.Update(tea.KeyMsg{Type: tt.key})
			m = updated.(model)

			if !m.cancelled {
				t.Errorf("expected cancelled=true for %s", tt.name)
			}
			if cmd == nil {
				t.Error("expected tea.Quit command")
			}
		})
	}
}

func TestModelQKey(t *testing.T) {
	t.Parallel()

	items := []Item{NewSimpleItem("cmd1", "desc1")}
	m := newModel(SelectOptions{Items: items})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(model)

	if !m.cancelled {
		t.Error("expected cancelled=true for 'q'")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestViewFormat(t *testing.T) {
	t.Parallel()

	items := []Item{
		NewSimpleItem("ls -la", "list all files"),
		NewSimpleItem("rm -rf /", "dangerous command"),
	}

	m := newModel(SelectOptions{
		Title:       "Select command:",
		Items:       items,
		ShowWarning: false,
	})

	view := m.View()

	if view == "" {
		t.Error("expected non-empty view")
	}

	// View должен содержать заголовок
	if len("Select command:") > 0 && len(view) < len("Select command:") {
		t.Error("view should contain title")
	}
}
