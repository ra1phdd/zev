package list

import "github.com/ra1phdd/zev-go/internal/core/domain"

// CommandItem адаптер для domain.Command.
type CommandItem struct {
	Cmd *domain.Command
}

func (i CommandItem) Title() string {
	return i.Cmd.Command
}

func (i CommandItem) Description() string {
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
