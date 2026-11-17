package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRussianHelpKeepsCommandTokens(t *testing.T) {
	t.Setenv("ZEV_LANG", "ru")
	root := &cobra.Command{Use: "zev"}
	root.AddCommand(newQueryCmd())
	localizeCommandHelp(root)
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Использование:", "Доступные команды:", historyValidationQuery, "--help"} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("help missing %q: %s", expected, out.String())
		}
	}
}
