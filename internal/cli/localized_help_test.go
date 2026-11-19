package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/locale"
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

//nolint:paralleltest // The fixture changes HOME and the process-wide UI language.
func TestNewRootCommandUsesPersistedRussianLanguageForHelp(t *testing.T) {
	home := newCLIConfigHome(t)
	setFixtureLanguage(t, home, "ru")
	restore := locale.SetLanguage("en", "")
	t.Cleanup(restore)

	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{entityHelpFlag})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Использование:") {
		t.Fatalf("root help was not localized: %q", out.String())
	}
}
