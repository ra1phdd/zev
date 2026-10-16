package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev-go/internal/core/config"
	"github.com/ra1phdd/zev-go/internal/entities/script/repository"
	"github.com/ra1phdd/zev-go/internal/entities/script/service"
)

func setupTestEnv(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	oldGetZevDir := config.GetZevDir
	config.GetZevDir = func() string {
		return tmpDir
	}

	cleanup := func() {
		config.GetZevDir = oldGetZevDir
	}

	return tmpDir, cleanup
}

func TestServiceCreate(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Пропускаем открытие редактора в тестах
	t.Setenv("SKIP_EDITOR", "1")

	repo := repository.New()
	svc := service.New(repo)

	// Создаём скрипт
	err := svc.Create("test", "Test script")
	if err != nil {
		t.Fatalf("Failed to create script: %v", err)
	}

	// Проверяем, что файл создан
	scriptPath := filepath.Join(tmpDir, "scripts", "test.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Error("Script file was not created")
	}

	// Проверяем метаданные
	scripts, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list scripts: %v", err)
	}

	if len(scripts) != 1 {
		t.Fatalf("Expected 1 script, got %d", len(scripts))
	}

	if scripts[0].Name != "test" || scripts[0].Description != "Test script" {
		t.Errorf("Unexpected script metadata: %+v", scripts[0])
	}
}

func TestServiceRemove(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	t.Setenv("SKIP_EDITOR", "1")

	repo := repository.New()
	svc := service.New(repo)

	// Создаём и удаляем скрипт
	if err := svc.Create("test", "Test script"); err != nil {
		t.Fatalf("Failed to create script: %v", err)
	}

	if err := svc.Remove("test"); err != nil {
		t.Fatalf("Failed to remove script: %v", err)
	}

	// Проверяем, что файл удалён
	scriptPath := filepath.Join(tmpDir, "scripts", "test.sh")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("Script file was not removed")
	}

	// Проверяем, что метаданные удалены
	scripts, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list scripts: %v", err)
	}

	if len(scripts) != 0 {
		t.Errorf("Expected 0 scripts, got %d", len(scripts))
	}
}

func TestServiceList(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	t.Setenv("SKIP_EDITOR", "1")

	repo := repository.New()
	svc := service.New(repo)

	// Создаём несколько скриптов
	if err := svc.Create("script1", "First script"); err != nil {
		t.Fatalf("Failed to create script1: %v", err)
	}
	if err := svc.Create("script2", "Second script"); err != nil {
		t.Fatalf("Failed to create script2: %v", err)
	}

	scripts, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list scripts: %v", err)
	}

	if len(scripts) != 2 {
		t.Errorf("Expected 2 scripts, got %d", len(scripts))
	}
}
