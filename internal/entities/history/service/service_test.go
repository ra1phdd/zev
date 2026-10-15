package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ra1phdd/zev-go/internal/core/domain"
)

func TestHistorySaveAndLoad(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "test.history")

	// Переопределяем путь для теста
	srv := &Service{historyPath: historyPath}

	entry := HistoryEntry{
		Timestamp: time.Now(),
		Prompt:    "list files",
		Response: &domain.OptionsResponse{
			Commands: []domain.Command{
				{
					Command:          "ls -la",
					ShortExplanation: "List all files",
					IsDangerous:      false,
				},
			},
			IsValid: true,
		},
		Provider: "openai",
		Model:    "gpt-4-turbo",
	}

	// Сохраняем
	if err := srv.Save(entry); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Загружаем
	entries, err := srv.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Prompt != "list files" {
		t.Errorf("expected prompt='list files', got %s", entries[0].Prompt)
	}
	if entries[0].Provider != "openai" {
		t.Errorf("expected provider=openai, got %s", entries[0].Provider)
	}
}

func TestHistoryRotation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "test.history")

	srv := &Service{historyPath: historyPath}

	// Создаём больше maxHistoryEntries записей
	for i := range maxHistoryEntries + 10 {
		entry := HistoryEntry{
			Timestamp: time.Now(),
			Prompt:    "test prompt",
			Response: &domain.OptionsResponse{
				Commands: []domain.Command{
					{Command: "echo test", ShortExplanation: "test", IsDangerous: false},
				},
				IsValid: true,
			},
			Provider: "openai",
			Model:    "gpt-4",
		}
		if err := srv.Save(entry); err != nil {
			t.Fatalf("Save failed at iteration %d: %v", i, err)
		}
	}

	// Проверяем, что осталось ровно maxHistoryEntries
	entries, err := srv.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(entries) != maxHistoryEntries {
		t.Errorf("expected %d entries after rotation, got %d", maxHistoryEntries, len(entries))
	}
}

func TestHistoryLoadEmpty(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "nonexistent.history")

	srv := &Service{historyPath: historyPath}

	entries, err := srv.LoadAll()
	if err == nil {
		t.Error("expected error when loading nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}
