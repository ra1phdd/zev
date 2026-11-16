package service_test

import (
	"errors"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

func TestAddCommandPreservesLiteralTemplates(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`docker inspect --format '{{.State.Status}}' container`,
		`printf '{{.args}}'`,
		`printf '{{unfinished'`,
		`printf '}}'`,
		`printf 'hello: world' | cat`,
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(t)
			if err := svc.AddCommand("literal", command); err != nil {
				t.Fatal(err)
			}
			got, err := svc.Resolve("literal", nil)
			if err != nil || got != command {
				t.Fatalf("Resolve=%q err=%v, want %q", got, err, command)
			}
			if err := svc.AddCommand("literal", "echo overwritten"); !errors.Is(err, service.ErrAliasExists) {
				t.Fatalf("conflict=%v", err)
			}
			got, err = svc.Resolve("literal", nil)
			if err != nil || got != command {
				t.Fatalf("conflict overwrote alias: %q, %v", got, err)
			}
		})
	}
}
