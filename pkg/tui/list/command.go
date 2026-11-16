package list

import (
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// CommandItem adapts a non-nil domain command for list display.
type CommandItem struct {
	Cmd *domain.Command
}

// Title returns the command, or its explanation for an internal action.
func (i CommandItem) Title() string {
	if strings.HasPrefix(i.Cmd.Command, "::") {
		return i.Cmd.ShortExplanation
	}
	return i.Cmd.Command
}

// Description returns the explanation, or an empty string for internal actions.
func (i CommandItem) Description() string {
	if strings.HasPrefix(i.Cmd.Command, "::") {
		return ""
	}
	return i.Cmd.ShortExplanation
}

// IsWarning reports whether the command is marked dangerous.
func (i CommandItem) IsWarning() bool {
	return i.Cmd.IsDangerous
}

// FilterValue returns the command text used for filtering.
func (i CommandItem) FilterValue() string {
	return i.Cmd.Command
}

// NewCommandItem creates a list entry for cmd, which must not be nil.
func NewCommandItem(cmd *domain.Command) CommandItem {
	return CommandItem{Cmd: cmd}
}
