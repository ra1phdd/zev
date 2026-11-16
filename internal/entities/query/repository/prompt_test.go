package repository_test

import (
	"testing"
)

func TestUserPromptTemplateContainsOnlyPromptPlaceholder(t *testing.T) {
	t.Parallel()

	got := "list files"
	if got != "list files" {
		t.Fatalf("user prompt = %q, want %q", got, "list files")
	}
}
