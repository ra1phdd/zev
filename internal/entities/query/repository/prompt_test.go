package repository_test

import (
	"testing"

	"github.com/ra1phdd/zev/internal/entities/query/repository"
)

func TestUserPromptTemplateContainsOnlyPromptPlaceholder(t *testing.T) {
	t.Parallel()

	if repository.UserPromptTemplate != "%s" {
		t.Fatalf("user prompt template = %q, want only %%s placeholder", repository.UserPromptTemplate)
	}
}
