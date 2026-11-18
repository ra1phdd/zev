package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/repository"
	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

func TestResolveClassifiesOnlyMissingAliases(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		content string
		missing bool
	}{
		{name: "missing file", missing: true},
		{name: "missing entry", content: "version: 2\naliases: {other: {template: echo ok}}\n", missing: true},
		{name: "corrupt YAML", content: "broken: [\n"},
		{name: "invalid template", content: "version: 2\naliases: {test: {template: '{{.arg1'}}\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.content != "" {
				if err := os.WriteFile(filepath.Join(dir, "aliases"), []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			resolved, err := service.New(repository.New(dir)).Resolve("test", nil)
			if err == nil || resolved != "" {
				t.Fatalf("Resolve = %q, %v; want error", resolved, err)
			}
			if errors.Is(err, service.ErrAliasNotFound) != tt.missing {
				t.Fatalf("Resolve error = %v, missing = %v", err, tt.missing)
			}
		})
	}
}
