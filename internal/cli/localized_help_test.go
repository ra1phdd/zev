package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
)

const (
	queryCommandName          = "query"
	continueCommandName       = "continue"
	allowSensitiveContextFlag = "--allow-sensitive-context"
	contextFileFlag           = "--context-file"
	continueFlagName          = "--continue"
	executeFlagName           = "--execute"
	injectFlagName            = "--inject"
	jsonFlagName              = "--json"
	optionFlagName            = "--option"
	printFlagName             = "--print"
	toolApprovalFlagName      = "--tool-approval"
	toolsFlagName             = "--tools"
	verboseFlagName           = "--verbose"
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

//nolint:paralleltest // The fixture changes HOME and process-wide language.
func TestRussianRunHelpUsesOwnedTemplate(t *testing.T) {
	home := newCLIConfigHome(t)
	setFixtureLanguage(t, home, "ru")
	restore := locale.SetLanguage("en", "")
	t.Cleanup(restore)

	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"run", entityHelpFlag})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Использование:", "Параметры:", "--on-failure=ask|none|analyze|fix", "--no-interactive"} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("Russian run help missing %q: %s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "Usage: zev run") {
		t.Errorf("Russian run help retained English template: %s", out.String())
	}
}

//nolint:paralleltest // Help output depends on process-wide locale state.
func TestQueryHelpListsManuallyParsedFlags(t *testing.T) {
	home := newCLIConfigHome(t)
	setFixtureLanguage(t, home, "en")
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{entityHelpFlag}, want: []string{executeFlagName, injectFlagName, continueFlagName, printFlagName, jsonFlagName, optionFlagName, contextFileFlag, allowSensitiveContextFlag, toolsFlagName, toolDisableFlag, toolApprovalFlagName, cliProxyFlag, verboseFlagName}},
		{name: queryCommandName, args: []string{queryCommandName, entityHelpFlag}, want: []string{executeFlagName, injectFlagName, continueFlagName, printFlagName, jsonFlagName, optionFlagName, contextFileFlag, allowSensitiveContextFlag, toolsFlagName, toolDisableFlag, toolApprovalFlagName, cliProxyFlag, verboseFlagName}},
		{name: continueCommandName, args: []string{continueCommandName, entityHelpFlag}, want: []string{continueFlagName, "--no-history", "--no-context", printFlagName, jsonFlagName, optionFlagName, contextFileFlag, allowSensitiveContextFlag, toolsFlagName, toolDisableFlag, toolApprovalFlagName, cliProxyFlag, verboseFlagName}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand("test")
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, flag := range tc.want {
				if !strings.Contains(out.String(), flag) {
					t.Errorf("help missing %q: %s", flag, out.String())
				}
			}
		})
	}
}
