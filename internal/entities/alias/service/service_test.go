package service_test

import (
	"errors"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/repository"
	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

func newTestService(t *testing.T) *service.Service {
	t.Helper()
	return service.New(repository.New(t.TempDir()))
}

func TestServiceAddAndResolve(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.Add("dps", "docker service ps {{.arg1}} --no-trunc"); err != nil {
		t.Fatal(err)
	}
	resolved, err := svc.Resolve("dps", []string{"backend"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "docker service ps 'backend' --no-trunc"; resolved != want {
		t.Fatalf("resolved=%q want=%q", resolved, want)
	}
}

func TestServiceResolveMultipleArgs(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.Add("dcup", "docker-compose up -d {{.args}}"); err != nil {
		t.Fatal(err)
	}
	resolved, err := svc.Resolve("dcup", []string{"web", "db", "redis"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "docker-compose up -d 'web' 'db' 'redis'"; resolved != want {
		t.Fatalf("resolved=%q want=%q", resolved, want)
	}
}

func TestServiceRejectsUnsupportedShellSyntax(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.Add("pipeline", "printf hello | cat"); err == nil {
		t.Fatal("unsupported shell syntax accepted")
	}
}

func TestServiceRemove(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.Add("test", "echo test"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove("test"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve("test", nil); !errors.Is(err, service.ErrAliasNotFound) {
		t.Fatalf("Resolve error=%v, want ErrAliasNotFound", err)
	}
}

func TestServiceList(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	for _, name := range []string{"alias1", "alias2"} {
		if err := svc.Add(name, "echo "+name); err != nil {
			t.Fatal(err)
		}
	}
	aliases, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 2 || aliases[0].Name != "alias1" || aliases[1].Name != "alias2" {
		t.Fatalf("aliases=%#v", aliases)
	}
}
