package cli

import (
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	return newHistoryCmdWithOpener(openHistory)
}

func newHistoryCmdWithOpener(open historyOpener) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "history",
		Short: locale.Text(locale.KeyCliShowQueryHistory),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonOutput {
				return historyBrowseWithJSON(open, true)(cmd, nil)
			}
			return historyBrowse(open)(cmd, nil)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, locale.Text(locale.KeyCliOutputJSON))
	for _, child := range newHistoryCommands(openHistory) {
		cmd.AddCommand(child)
	}
	return cmd
}
