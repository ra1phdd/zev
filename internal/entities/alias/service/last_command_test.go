package service_test

import (
	"errors"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

func TestAddCommandStoresCanonicalArgvAlias(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.AddCommand("literal", "printf hello"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Resolve("literal", nil)
	if err != nil || got != "printf hello" {
		t.Fatalf("Resolve=%q err=%v", got, err)
	}
	if err := svc.AddCommand("literal", "echo overwritten"); !errors.Is(err, service.ErrAliasExists) {
		t.Fatalf("conflict=%v", err)
	}
}

func TestAddCommandRejectsUnsupportedPreviousShellCommand(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"printf 'hello: world' | cat",
		"printf $(touch marker)",
		"echo > output",
	} {
		t.Run(command, func(t *testing.T) {
			svc := newTestService(t)
			if err := svc.AddCommand("literal", command); err == nil {
				t.Fatal("unsupported shell command accepted")
			}
		})
	}
}
