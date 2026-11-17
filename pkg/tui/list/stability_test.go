package list

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ra1phdd/zev/internal/core/domain"
)

func TestWrapPreservesRussianText(t *testing.T) {
	t.Parallel()
	const text = "Приветмир"
	got := wrapText(text, 3, 1)
	if !utf8.ValidString(got) {
		t.Fatalf("wrap split a UTF-8 character: %q", got)
	}
	if strings.ReplaceAll(strings.ReplaceAll(got, "\n", ""), " ", "") != text {
		t.Fatalf("wrap lost text: %q", got)
	}
}

func TestViewHandlesVeryNarrowTerminal(t *testing.T) {
	t.Parallel()
	m := newModel(SelectOptions{Items: []Item{NewSimpleItem("long command", "long description")}})
	m.width = 1
	if view := m.View(); view == "" {
		t.Fatal("empty view")
	}
}

func TestCommandItemOnlyHidesExactRefinementMarker(t *testing.T) {
	t.Parallel()
	item := NewCommandItem(&domain.Command{Command: "::literal", ShortExplanation: "explanation"})
	if item.Title() != "::literal" || item.Description() != "explanation" {
		t.Fatalf("literal command hidden: %q / %q", item.Title(), item.Description())
	}
}
