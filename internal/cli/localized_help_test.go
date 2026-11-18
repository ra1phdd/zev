package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

//nolint:paralleltest // The fixture changes HOME and the process-wide UI language.
func TestRussianHelpKeepsCommandTokens(t *testing.T) {
	setFixtureLanguage(t, newCLIConfigHome(t), "ru")
	root := &cobra.Command{Use: "zev"}
	root.AddCommand(newQueryCmd())
	localizeCommandHelp(root)
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Использование:", "Доступные команды:", historyValidationQuery, entityHelpFlag} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("help missing %q: %s", expected, out.String())
		}
	}
}
