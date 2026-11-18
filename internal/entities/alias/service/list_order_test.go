package service

import (
	"slices"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/repository"
)

func TestAliasListHasStableNameOrder(t *testing.T) {
	t.Parallel()
	svc := New(repository.New(t.TempDir()))
	for _, name := range []string{"zeta", "alpha", "middle"} {
		if err := svc.Add(name, "echo fixture"); err != nil {
			t.Fatal(err)
		}
	}
	aliases, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(aliases))
	for i, alias := range aliases {
		names[i] = alias.Name
	}
	if !slices.Equal(names, []string{"alpha", "middle", "zeta"}) {
		t.Fatalf("alias order = %q", names)
	}
}
