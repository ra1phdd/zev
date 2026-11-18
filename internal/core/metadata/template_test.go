package metadata

import (
	"strings"
	"testing"
)

func TestTemplateDiagnosticsDoNotEchoInvalidWords(t *testing.T) {
	t.Parallel()
	secret := "SECRET_TOKEN_VALUE"
	for _, tc := range []struct {
		name     string
		template string
		category string
	}{
		{name: "glob", template: "echo " + secret + "*", category: "shell glob syntax"},
		{name: "tilde", template: "echo ~" + secret, category: "tilde expansion"},
		{name: "assignment", template: "echo " + secret + "=value", category: "shell assignment"},
		{name: "placeholder", template: "echo " + secret + "{{.arg1}}", category: "non-standalone placeholder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAliasTemplate(tc.template, nil)
			if err == nil {
				t.Fatal("invalid template accepted")
			}
			message := err.Error()
			if strings.Contains(message, secret) {
				t.Fatalf("diagnostic echoed secret: %q", message)
			}
			if !strings.Contains(message, tc.category) || !strings.Contains(message, "word 2") {
				t.Fatalf("diagnostic = %q, want category %q and word position", message, tc.category)
			}
		})
	}
}

func TestTemplateRejectsShellReevaluationAndReservedWords(t *testing.T) {
	t.Parallel()
	for _, template := range []string{"eval {{.arg1}}", "bash -c {{.arg1}}", "function foo", "coproc foo"} {
		t.Run(template, func(t *testing.T) {
			if err := ValidateAliasTemplate(template, nil); err == nil {
				t.Fatalf("template %q was accepted", template)
			}
		})
	}
}
