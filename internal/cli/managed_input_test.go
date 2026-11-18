package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestManagedInputPreservesPrefillAndCancels(t *testing.T) {
	t.Parallel()
	model := newManagedInput("Description", "current value")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result, ok := updated.(managedInput)
	if !ok || !result.submitted || result.value != "current value" {
		t.Fatalf("prefill lost: %#v", updated)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	result, ok = updated.(managedInput)
	if !ok || !result.cancelled {
		t.Fatalf("Escape did not cancel: %#v", updated)
	}
}
