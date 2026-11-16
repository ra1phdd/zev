package main

import "github.com/spf13/cobra"

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show query history",
		Args:  cobra.NoArgs,
		RunE:  historyBrowse(openHistory),
	}
	for _, child := range newHistoryCommands(openHistory) {
		cmd.AddCommand(child)
	}
	return cmd
}
