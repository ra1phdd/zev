// Package service stores and rotates the command execution history.
package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const maxHistoryEntries = 100

// HistoryEntry records one completed command request.
type HistoryEntry struct {
	Timestamp       time.Time               `json:"timestamp"`
	Prompt          string                  `json:"prompt"`
	Response        *domain.OptionsResponse `json:"response"`
	Provider        string                  `json:"provider"`
	Model           string                  `json:"model"`
	SelectedCommand string                  `json:"selected_command,omitempty"` //nolint:tagliatelle // Snake_case is the persisted history JSON format.
	SelectedIndex   int                     `json:"selected_index,omitempty"`   //nolint:tagliatelle // Snake_case is the persisted history JSON format.
}

// Service manages the persisted command history.
type Service struct {
	historyPath string
}

// New creates a history service using the user's Zev directory.
func New() *Service {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
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

// LoadAll reads all valid entries from the history file.
func (s *Service) LoadAll() (entries []HistoryEntry, err error) {
	file, err := os.Open(s.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err //nolint:wrapcheck // Callers use os.IsNotExist to handle a missing history file.
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close history file: %w", closeErr))
		}
	}()

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

// saveAll rewrites the history file with entries.
func (s *Service) saveAll(entries []HistoryEntry) (err error) {
	file, err := os.Create(s.historyPath)
	if err != nil {
		return fmt.Errorf("failed to create history file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close history file: %w", closeErr))
		}
	}()

	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("failed to write history entry: %w", err)
		}
	}

	return nil
}
