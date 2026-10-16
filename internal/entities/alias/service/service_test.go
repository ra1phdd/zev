package service_test

import (
	"testing"

	"github.com/ra1phdd/zev/internal/entities/alias/repository"
	"github.com/ra1phdd/zev/internal/entities/alias/service"
)

// newTestService создаёт сервис алиасов с репозиторием во временной директории.
// t.TempDir() уникален для каждого теста, поэтому тесты изолированы и не
// разделяют глобальное состояние.
func newTestService(t *testing.T) *service.Service {
	t.Helper()

	return service.New(repository.New(t.TempDir()))
}

func TestServiceAddAndResolve(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)

	// Добавляем алиас
	err := svc.Add("dps", "docker service ps {{.arg1}} --no-trunc")
	if err != nil {
		t.Fatalf("Failed to add alias: %v", err)
	}

	// Проверяем resolve
	resolved, err := svc.Resolve("dps", []string{"backend"})
	if err != nil {
		t.Fatalf("Failed to resolve alias: %v", err)
	}

	expected := "docker service ps backend --no-trunc"
	if resolved != expected {
		t.Errorf("Expected %q, got %q", expected, resolved)
	}
}

func TestServiceResolveMultipleArgs(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)

	// Алиас с {{.args}}
	err := svc.Add("dcup", "docker-compose up -d {{.args}}")
	if err != nil {
		t.Fatalf("Failed to add alias: %v", err)
	}

	resolved, err := svc.Resolve("dcup", []string{"web", "db", "redis"})
	if err != nil {
		t.Fatalf("Failed to resolve alias: %v", err)
	}

	expected := "docker-compose up -d web db redis"
	if resolved != expected {
		t.Errorf("Expected %q, got %q", expected, resolved)
	}
}

func TestServiceRemove(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)

	// Добавляем и удаляем алиас
	if err := svc.Add("test", "echo test"); err != nil {
		t.Fatalf("Failed to add alias: %v", err)
	}

	if err := svc.Remove("test"); err != nil {
		t.Fatalf("Failed to remove alias: %v", err)
	}

	// Проверяем, что алиас удалён
	_, err := svc.Resolve("test", nil)
	if err == nil {
		t.Error("Expected error for removed alias, got nil")
	}
}

func TestServiceList(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)

	// Добавляем несколько алиасов
	if err := svc.Add("alias1", "cmd1"); err != nil {
		t.Fatalf("Failed to add alias1: %v", err)
	}
	if err := svc.Add("alias2", "cmd2"); err != nil {
		t.Fatalf("Failed to add alias2: %v", err)
	}

	aliases, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list aliases: %v", err)
	}

	if len(aliases) != 2 {
		t.Errorf("Expected 2 aliases, got %d", len(aliases))
	}
}

func TestServiceResolveAutoAppendArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		args     []string
		expected string
	}{
		{
			name:     "no template variables - args appended",
			template: "docker ps",
			args:     []string{"-a", "--format", "json"},
			expected: "docker ps -a --format json",
		},
		{
			name:     "no template variables - no args",
			template: "docker ps",
			args:     []string{},
			expected: "docker ps",
		},
		{
			name:     "has {{.args}} - no auto-append",
			template: "docker ps {{.args}}",
			args:     []string{"-a", "--format", "json"},
			expected: "docker ps -a --format json",
		},
		{
			name:     "has {{.arg1}} - no auto-append",
			template: "docker logs {{.arg1}}",
			args:     []string{"container123"},
			expected: "docker logs container123",
		},
		{
			name:     "has multiple {{.argN}} - no auto-append",
			template: "scp {{.arg1}} {{.arg2}}",
			args:     []string{"local.txt", "remote:/path"},
			expected: "scp local.txt remote:/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := newTestService(t)

			// Добавляем алиас
			aliasName := "test_" + tt.name
			err := svc.Add(aliasName, tt.template)
			if err != nil {
				t.Fatalf("Failed to add alias: %v", err)
			}

			// Разрешаем алиас
			resolved, err := svc.Resolve(aliasName, tt.args)
			if err != nil {
				t.Fatalf("Failed to resolve alias: %v", err)
			}

			if resolved != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, resolved)
			}
		})
	}
}
