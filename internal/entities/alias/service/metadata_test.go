package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/alias/repository"
	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

func TestResolvePreservesExactArgvWithoutExecution(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.Add("show", "printf %s {{.args}}"); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "executed")
	argv := []string{"", "a b", "\t\n", "тест", "a'b", "$HOME", "*", "; touch " + marker, "&&", "$(touch " + marker + ")", "`touch " + marker + "`"}
	got, err := svc.Resolve("show", argv)
	if err != nil {
		t.Fatal(err)
	}
	want := "printf %s '' 'a b' '\t\n' 'тест' 'a'\"'\"'b' '$HOME' '*' '; touch " + marker + "' '&&' '$(touch " + marker + ")' '`touch " + marker + "`'"
	if got != want {
		t.Fatalf("rendered=%q want=%q", got, want)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Resolve executed command: %v", err)
	}
}

func TestResolveRejectsMissingAndExtraArguments(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.Add("show", "echo {{.arg1}}"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{name: "missing", want: "requires argument 1"},
		{name: "extra", argv: []string{"one", "two"}, want: "accepts at most 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.Resolve("show", tc.argv)
			if err == nil || !strings.Contains(err.Error(), tc.want) || got != "" {
				t.Fatalf("Resolve=%q err=%v want=%q", got, err, tc.want)
			}
		})
	}
	if got, err := svc.Resolve("show", []string{""}); err != nil || got != "echo ''" {
		t.Fatalf("supplied empty argument = %q err=%v", got, err)
	}
}

func TestAliasMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo := repository.New(dir)
	want := domain.Alias{Name: "show", Template: "echo {{.arg1}} {{.arg2}}", Description: "show values", Arguments: []domain.Argument{
		{Name: "value", Description: "required text", Required: true},
		{Name: "suffix", Description: "optional suffix", Required: false},
	}}
	if err := repo.Update(func(aliases map[string]domain.Alias) error { aliases[want.Name] = want; return nil }); err != nil {
		t.Fatal(err)
	}
	svc := service.New(repository.New(dir))
	got, err := svc.List()
	if err != nil || !reflect.DeepEqual(got, []domain.Alias{want}) {
		t.Fatalf("List=%#v err=%v", got, err)
	}
	if resolved, err := svc.Resolve(want.Name, []string{"hello world"}); err != nil || resolved != "echo 'hello world'" {
		t.Fatalf("optional argument resolved=%q err=%v", resolved, err)
	}
}
