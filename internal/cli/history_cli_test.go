package cli

import (
	"io"
	"strings"
	"testing"
)

const historyValidationQuery = "query"

func TestHistoryCommandValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"empty search", []string{historyCommandSearch, " "}, historyValidationQuery},
		{"unknown mode", []string{historyCommandSearch, historyTestFiles, "--mode", historyBadValue}, "mode"},
		{"unknown fuzzy", []string{historyCommandSearch, historyTestFiles, "--fuzzy", historyBadValue}, "fuzzy"},
		{"negative limit", []string{historyCommandSearch, historyTestFiles, "--limit", "-1"}, "limit"},
		{"bad date", []string{historyCommandSearch, historyTestFiles, "--from", historyBadValue}, "from"},
		{"bad type", []string{historyCommandSearch, historyTestFiles, "--type", historyBadValue}, "type"},
		{"show no id", []string{historyCommandShow}, "id"},
		{"show conflicting ids", []string{historyCommandShow, "conversation", "--message", "message"}, "id"},
		{"export format", []string{"export", historyFormatFlag, historyBadValue}, "format"},
		{"delete no id", []string{"delete"}, "arg"},
		{"clear needs confirmation", []string{historyCommandClear}, historyYesFlag},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newHistoryCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}
