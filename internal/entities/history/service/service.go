package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ra1phdd/zev-go/internal/core/domain"
)

const maxHistoryEntries = 100

type HistoryEntry struct {
	Timestamp        time.Time               `json:"timestamp"`
	Prompt           string                  `json:"prompt"`
	Response         *domain.OptionsResponse `json:"response"`
	Provider         string                  `json:"provider"`
	Model            string                  `json:"model"`
	SelectedCommand  string                  `json:"selected_command,omitempty"`  // команда, которую выбрал пользователь
	SelectedIndex    int                     `json:"selected_index,omitempty"`    // индекс выбранной команды (для highlight)
}

type Service struct {
	historyPath string
}

func New() *Service {
	home, _ := os.UserHomeDir()
	return &Service{
		historyPath: filepath.Join(home, ".zev", "history"),
	}
}

// Save сохраняет запись в историю с автоматической ротацией.
func (s *Service) Save(entry HistoryEntry) error {
	entries, err := s.LoadAll()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load existing history: %w", err)
	}

	entries = append(entries, entry)

	// Ротация: оставляем только последние maxHistoryEntries записей
	if len(entries) > maxHistoryEntries {
		entries = entries[len(entries)-maxHistoryEntries:]
	}

	return s.saveAll(entries)
}

func (s *Service) LoadAll() ([]HistoryEntry, error) {
	file, err := os.Open(s.historyPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read history: %w", err)
	}

	return entries, nil
}

// saveAll сохраняет все записи в файл (перезаписывает).
func (s *Service) saveAll(entries []HistoryEntry) error {
	file, err := os.Create(s.historyPath)
	if err != nil {
		return fmt.Errorf("failed to create history file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("failed to write history entry: %w", err)
		}
	}

	return nil
}
