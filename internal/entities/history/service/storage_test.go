package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryCorruptionPreservesFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "history")
	original := []byte("{\"prompt\":\"valid\"}\nnot json\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Service{historyPath: path}
	if _, err := s.LoadAll(); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("LoadAll error = %v, want corruption at line 2", err)
	}
	if err := s.Save(HistoryEntry{Prompt: "new"}); err == nil {
		t.Error("Save accepted corrupt history")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Errorf("corrupt history was overwritten: %q", data)
	}
}

func TestHistoryEncodingFailurePreservesFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "history")
	s := &Service{historyPath: path}
	if err := s.Save(HistoryEntry{Prompt: "original"}); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.saveAll([]HistoryEntry{{Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}, {Prompt: "replacement"}}); err == nil {
		t.Fatal("Save accepted an unencodable timestamp")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Errorf("history changed after encoding failure: %q", data)
	}
}

func TestHistoryConcurrentSave(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "history")
	const count = 40
	errs := make(chan error, count)
	start := make(chan struct{})
	for i := range count {
		go func() {
			<-start
			s := &Service{historyPath: path}
			errs <- s.Save(HistoryEntry{Prompt: fmt.Sprintf("entry-%d", i)})
		}()
	}
	close(start)
	for range count {
		if err := <-errs; err != nil {
			t.Errorf("Save: %v", err)
		}
	}
	entries, err := (&Service{historyPath: path}).LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		seen[entry.Prompt] = true
	}
	if len(seen) != count {
		t.Errorf("got %d unique entries, want %d", len(seen), count)
	}
}
