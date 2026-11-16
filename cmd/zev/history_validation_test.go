package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/domain"
	historyservice "github.com/ra1phdd/zev/internal/entities/history/service"
)

func TestHistoryRejectsMalformedCommandsBeforeSelection(t *testing.T) {
	tests := []struct {
		name string
		edit func(*historyservice.HistoryEntry)
	}{
		{name: "empty command", edit: func(e *historyservice.HistoryEntry) { e.Response.Commands[0].Command = "" }},
		{name: "whitespace command", edit: func(e *historyservice.HistoryEntry) { e.Response.Commands[0].Command = "   " }},
		{name: "NUL command", edit: func(e *historyservice.HistoryEntry) { e.Response.Commands[0].Command = "echo \x00" }},
		{name: "ESC command", edit: func(e *historyservice.HistoryEntry) { e.Response.Commands[0].Command = "echo \x1b[2J" }},
		{name: "explanation control", edit: func(e *historyservice.HistoryEntry) { e.Response.Commands[0].ShortExplanation = "\x1b[2J" }},
		{name: "oversized command", edit: func(e *historyservice.HistoryEntry) {
			e.Response.Commands[0].Command = strings.Repeat("x", domain.MaxCommandLength+1)
		}},
		{name: "too many commands", edit: func(e *historyservice.HistoryEntry) {
			e.Response.Commands = []domain.Command{{Command: "ls"}, {Command: "pwd"}, {Command: "date"}, {Command: "whoami"}}
		}},
		{name: "invalid response with commands", edit: func(e *historyservice.HistoryEntry) { e.Response.IsValid = false }},
		{name: "empty valid response", edit: func(e *historyservice.HistoryEntry) { e.Response.Commands = nil }},
		{name: "selected command control", edit: func(e *historyservice.HistoryEntry) { e.SelectedCommand = "echo \x1b[2J" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := historyservice.HistoryEntry{
				Response: &domain.OptionsResponse{IsValid: true, Commands: []domain.Command{{Command: "echo safe"}}},
			}
			tt.edit(&entry)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			var out bytes.Buffer
			if err := showHistoryDetails(ctx, &entry, &out); !errors.Is(err, domain.ErrInvalidOptionsResponse) {
				t.Errorf("showHistoryDetails() error = %v, want validation before selection", err)
			}
			if out.Len() != 0 {
				t.Errorf("malformed entry produced output: %q", out.String())
			}

			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			dir := filepath.Join(home, ".zev")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "history"), append(data, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := newHistoryCmd()
			cmd.SetContext(ctx)
			cmd.SetOut(io.Discard)
			if err := runHistory(cmd, nil); !errors.Is(err, domain.ErrInvalidOptionsResponse) {
				t.Errorf("runHistory() error = %v, want validation before history selection", err)
			}
		})
	}
}

func TestHistoryPreservesSafeLegacyEmptyEntries(t *testing.T) {
	t.Parallel()
	for _, response := range []*domain.OptionsResponse{nil, {IsValid: false, ExplanationIfNotValid: "unsupported request"}} {
		var out bytes.Buffer
		if err := showHistoryDetails(t.Context(), &historyservice.HistoryEntry{Response: response}, &out); err != nil {
			t.Fatal(err)
		}
		if out.String() != "No commands in this entry\n" {
			t.Errorf("legacy output = %q", out.String())
		}
	}
}

func TestHistoryAllowsMultilineResponseBeforeInjection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	entry := historyservice.HistoryEntry{Response: &domain.OptionsResponse{
		IsValid: true, Commands: []domain.Command{{Command: "echo first\necho second"}},
	}}
	if err := showHistoryDetails(ctx, &entry, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("showHistoryDetails() error = %v, want selection cancellation, not domain rejection", err)
	}
}
