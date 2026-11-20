package list

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const listTestTitle = "Select"

func TestSelectContextAlreadyCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := SelectContext(ctx, SelectOptions{Items: []Item{NewSimpleItem("item", "")}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SelectContext() error = %v, want context.Canceled", err)
	}
}

func TestSelectContextEOFReturnsWithoutSelection(t *testing.T) {
	t.Parallel()

	_, err := SelectContext(t.Context(), SelectOptions{
		Items: []Item{NewSimpleItem("item", "")},
		Input: strings.NewReader(""),
	})
	if err == nil {
		t.Fatal("SelectContext() accepted EOF without selection")
	}
}

func TestSelectContextUsesProvidedInput(t *testing.T) {
	t.Parallel()

	selected, err := selectContext(t.Context(), SelectOptions{
		Items: []Item{NewSimpleItem("item", "")},
		Input: strings.NewReader("\r"),
	}, tea.WithoutSignalHandler())
	if err != nil {
		t.Fatalf("selectContext() error = %v", err)
	}
	if selected == nil || selected.Title() != "item" {
		t.Fatalf("selected = %#v", selected)
	}
}

func requireModel(t *testing.T, updated tea.Model) model {
	t.Helper()
	m, ok := updated.(model)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	return m
}

func TestModelIgnoredKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyTab},
		{Type: tea.KeyLeft},
		{Type: tea.KeyRunes, Runes: []rune{'x'}},
	} {
		m := newModel(SelectOptions{Items: []Item{NewSimpleItem("command", "")}})
		updated, cmd := m.Update(key)
		m = requireModel(t, updated)
		if m.cursor != 0 || m.cancelled || m.selected != nil || cmd != nil {
			t.Errorf("ignored key %v changed selection state: %+v", key, m)
		}
	}
}

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
	m = requireModel(t, updated)

	if m.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", m.cursor)
	}

	// Another down
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = requireModel(t, updated)

	if m.cursor != 2 {
		t.Errorf("expected cursor=2 after second down, got %d", m.cursor)
	}

	// Down at bottom wraps to top
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = requireModel(t, updated)

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
	m = requireModel(t, updated)

	if m.cursor != 2 {
		t.Errorf("expected cursor=2 after up from top, got %d", m.cursor)
	}

	// Up again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = requireModel(t, updated)

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
	m = requireModel(t, updated)

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
			m = requireModel(t, updated)

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
	m = requireModel(t, updated)

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

	if !strings.Contains(view, "Select command:") {
		t.Error("view should contain title")
	}
}

func TestViewShowsNavigationHint(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Items: []Item{NewSimpleItem("command", "")}})
	if view := m.View(); !strings.Contains(view, "↑/↓") || !strings.Contains(view, "Enter") || !strings.Contains(view, "Esc") {
		t.Fatalf("view lacks navigation hint: %q", view)
	}
}

func TestViewScrollsWithCursor(t *testing.T) {
	t.Parallel()

	items := make([]Item, 8)
	for i := range items {
		items[i] = NewSimpleItem(fmt.Sprintf("item-%d", i), "")
	}
	m := newModel(SelectOptions{Items: items})
	m.width = 80
	m.height = 6

	if view := m.View(); strings.Contains(view, "item-7") {
		t.Fatalf("initial view should be bounded: %q", view)
	}
	for range 7 {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = requireModel(t, updated)
	}
	view := m.View()
	if !strings.Contains(view, "item-7") {
		t.Fatalf("scrolled view does not contain selected item: %q", view)
	}
	if strings.Contains(view, "item-0") {
		t.Fatalf("scrolled view still contains first item: %q", view)
	}
}

func TestViewWrapsNavigationHintInNarrowTerminal(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Items: []Item{NewSimpleItem("item", "")}})
	m.width = 10
	m.height = 8
	view := strings.TrimSuffix(m.View(), "\n")
	for i, line := range strings.Split(view, "\n") {
		if len([]rune(line)) > m.width {
			t.Fatalf("line %d exceeds terminal width: %q", i, line)
		}
	}
}

func TestViewFitsHeightForLongSingleItem(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Title: listTestTitle, Items: []Item{
		NewSimpleItem(strings.Repeat("long item ", 20), strings.Repeat("description ", 20)),
	}})
	m.width = 10
	m.height = 3
	view := strings.TrimSuffix(m.View(), "\n")
	if lines := strings.Count(view, "\n") + 1; lines > m.height {
		t.Fatalf("view has %d lines for height %d: %q", lines, m.height, view)
	}
}

func TestViewUsesTerminalCellWidth(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Items: []Item{NewSimpleItem("界界界", "")}})
	m.width = 8
	for i, line := range strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("line %d exceeds terminal width: %d > %d: %q", i, ansi.StringWidth(line), m.width, line)
		}
	}
}

func TestViewFitsTerminalCellWidthBelowPrefixWidth(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Items: []Item{NewSimpleItem("item", "")}})
	m.width = 1
	for i, line := range strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("line %d exceeds terminal width: %d > %d: %q", i, ansi.StringWidth(line), m.width, line)
		}
	}
}

func TestViewDoesNotAddFooterWhenHeaderConsumesViewport(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Title: listTestTitle, Items: []Item{NewSimpleItem("item", "")}})
	m.width = 80
	m.height = 1
	view := strings.TrimSuffix(m.View(), "\n")
	if lines := strings.Count(view, "\n") + 1; lines > m.height {
		t.Fatalf("view has %d lines for height %d: %q", lines, m.height, view)
	}
}

func TestEnterDoesNotSelectHiddenItem(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Title: listTestTitle, Items: []Item{NewSimpleItem("item", "")}})
	m.width = 80
	m.height = 1
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := requireModel(t, updated)
	if got.selected != nil || cmd != nil {
		t.Fatalf("hidden item selected = %v, cmd = %v", got.selected, cmd)
	}
}

func TestViewPreservesExplicitLineBreaksWithIndentation(t *testing.T) {
	t.Parallel()

	m := newModel(SelectOptions{Items: []Item{NewSimpleItem("one\ntwo", "")}})
	if !strings.Contains(m.View(), "\n     two") {
		t.Fatalf("multiline item is not indented: %q", m.View())
	}
}
