// Package service stores and rotates the command execution history.
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/filestore"
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
	return &Service{historyPath: filepath.Join(home, ".zev", "history")}
}

// Save appends entry and retains the most recent 100 entries in one transaction.
// Corrupt history or failures before rename leave the previous file unchanged.
// Write errors preserve filestore.ErrCommitted when replacement succeeded but
// directory sync failed; callers must not blindly retry the append in that case.
func (s *Service) Save(entry HistoryEntry) error {
	if err := filestore.ValidateDirectory(filepath.Dir(s.historyPath)); err != nil {
		return fmt.Errorf("validate history directory: %w", err)
	}
	if err := filestore.WithLock(s.historyPath, func() error {
		entries, err := s.loadAll()
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to load existing history: %w", err)
		}
		entries = append(entries, entry)
		if len(entries) > maxHistoryEntries {
			entries = entries[len(entries)-maxHistoryEntries:]
		}
		return s.saveAll(entries)
	}); err != nil {
		return fmt.Errorf("save history: %w", err)
	}
	return nil
}

// LoadAll reads the history, returning an error with the line number on corruption.
// A missing file returns an error recognized by os.IsNotExist. Symlinks at the
// history file or application directory are rejected.
func (s *Service) LoadAll() (entries []HistoryEntry, err error) {
	if err := filestore.ValidateDirectory(filepath.Dir(s.historyPath)); err != nil {
		return nil, fmt.Errorf("validate history directory: %w", err)
	}
	err = filestore.WithLock(s.historyPath, func() error {
		entries, err = s.loadAll()
		return err
	})
	return entries, err
}

func (s *Service) loadAll() ([]HistoryEntry, error) {
	data, err := filestore.ReadFile(s.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err //nolint:wrapcheck // Callers use os.IsNotExist for missing history.
		}
		return nil, fmt.Errorf("read history file: %w", err)
	}
	var entries []HistoryEntry
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		if i == len(lines)-1 && len(line) == 0 {
			break
		}
		var entry HistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("corrupt history at line %d: %w", i+1, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Service) saveAll(entries []HistoryEntry) error {
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("failed to encode history entry: %w", err)
		}
	}
	if err := filestore.AtomicWrite(s.historyPath, data.Bytes(), 0o600); err != nil {
		return fmt.Errorf("failed to write history: %w", err)
	}
	return nil
}
