package cli

import (
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: locale.Text(locale.KeyCliShowQueryHistory),
		Args:  cobra.NoArgs,
		RunE:  historyBrowse(openHistory),
	}
	for _, child := range newHistoryCommands(openHistory) {
		cmd.AddCommand(child)
	}
	return cmd
}
