package prompt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRussianPromptRendersOnce(t *testing.T) {
	t.Parallel()
	const title = "Введите уточнение"
	m := newInputModel(title, false)
	if got := strings.Count(m.View(), title); got != 1 {
		t.Fatalf("prompt occurrences=%d want1", got)
	}
}

func TestPromptWrapPreservesUTF8(t *testing.T) {
	t.Parallel()
	const text = "Приветмир"
	got := wrapText(text, 3)
	if !utf8.ValidString(got) || strings.ReplaceAll(got, "\n", "") != text {
		t.Fatalf("corrupted Russian prompt: %q", got)
	}
}
