package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
)

func writeManagedList(cmd *cobra.Command, kind string, items []managedRecord) error {
	flag := cmd.Flags().Lookup("json")
	if flag != nil && flag.Value.String() == "true" {
		return writeManagedJSON(cmd, kind, nil, items)
	}
	empty, header := locale.KeyCliNoAliasesConfigured, locale.KeyCliNameTemplate
	if kind == managedKindScript {
		empty, header = locale.KeyCliNoScriptsConfigured, locale.KeyCliNameFileDescription
	}
	if len(items) == 0 {
		return writeQueryOutput(cmd.OutOrStdout(), "%s\n", locale.Text(empty))
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, locale.Text(header)); err != nil {
		return fmt.Errorf("write entity table header: %w", err)
	}
	for _, item := range items {
		source := item.Template
		if kind == managedKindScript {
			source = item.Script
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.Name, source, item.Description, item.usage); err != nil {
			return fmt.Errorf("write entity table: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush entity table: %w", err)
	}
	return nil
}
