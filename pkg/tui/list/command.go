package list

import (
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
)

// CommandItem адаптер для domain.Command.
type CommandItem struct {
	Cmd *domain.Command
}

func (i CommandItem) Title() string {
	if strings.HasPrefix(i.Cmd.Command, "::") {
		return i.Cmd.ShortExplanation
	}
	return i.Cmd.Command
}

func (i CommandItem) Description() string {
	if strings.HasPrefix(i.Cmd.Command, "::") {
		return ""
	}
	return i.Cmd.ShortExplanation
}

func (i CommandItem) IsWarning() bool {
	return i.Cmd.IsDangerous
}

func (i CommandItem) FilterValue() string {
	return i.Cmd.Command
}

// NewCommandItem создаёт элемент списка из domain.Command.
func NewCommandItem(cmd *domain.Command) CommandItem {
	return CommandItem{Cmd: cmd}
}
