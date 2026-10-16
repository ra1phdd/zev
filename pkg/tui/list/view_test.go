package list

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestViewWithWrapping(t *testing.T) {
	t.Parallel()

	items := []Item{
		NewSimpleItem(
			"find all python files recursively that are larger than 1 megabyte",
			"This command searches for Python files",
		),
	}

	m := newModel(SelectOptions{
		Title:       "Select a command:",
		Items:       items,
		ShowWarning: false,
	})

	// Установим узкую ширину для проверки переноса
	m.width = 40

	view := m.View()

	// Проверяем, что в выводе есть переносы строк
	lines := strings.Split(view, "\n")

	// Должно быть больше 3 строк: заголовок + пустая + команда (несколько строк) + описание
	if len(lines) < 5 {
		t.Errorf("Expected at least 5 lines with wrapping, got %d", len(lines))
	}

	// Проверяем, что ни одна строка не превышает ширину (с запасом на ANSI коды)
	for i, line := range lines {
		// Убираем ANSI escape codes для проверки реальной длины
		cleanLine := stripAnsi(line)
		if len(cleanLine) > m.width+5 { // +5 запас на префиксы
			t.Errorf("Line %d exceeds width limit: %d > %d\nLine: %q", i, len(cleanLine), m.width, cleanLine)
		}
	}
}

func TestViewWithWindowSizeMsg(t *testing.T) {
	t.Parallel()

	items := []Item{
		NewSimpleItem("Short", "Desc"),
	}

	m := newModel(SelectOptions{
		Title: "Test",
		Items: items,
	})

	// Проверяем дефолтную ширину
	if m.width != 80 {
		t.Errorf("Expected default width 80, got %d", m.width)
	}

	// Обновляем ширину через WindowSizeMsg
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = updated.(model)

	if m.width != 120 {
		t.Errorf("Expected width 120 after WindowSizeMsg, got %d", m.width)
	}
}

// stripAnsi убирает ANSI escape codes из строки для проверки реальной длины.
func stripAnsi(s string) string {
	// Простой strip для тестов
	result := strings.Builder{}
	inEscape := false

	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}
