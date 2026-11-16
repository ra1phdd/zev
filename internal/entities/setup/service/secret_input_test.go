package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAPIKeyPrompt(t *testing.T) {
	t.Parallel()

	const current = "saved-private-credential"
	const replacement = "replacement-private-credential"
	failure := errors.New("UI failure containing " + replacement)
	tests := []struct {
		name      string
		input     string
		cancelled bool
		failure   error
		want      string
		wantErr   bool
	}{
		{name: "retain", want: current},
		{name: "replace", input: "  " + replacement + "  ", want: replacement},
		{name: "cancel", input: replacement, cancelled: true, wantErr: true},
		{name: "UI failure", input: replacement, failure: failure, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Service{runSecretInput: func(model tea.Model) (tea.Model, error) {
				m, ok := model.(apiKeyInput)
				if !ok {
					t.Fatalf("unexpected model type %T", model)
				}
				if strings.Contains(m.View(), current) || !strings.Contains(m.View(), "press Enter to keep") {
					t.Fatal("saved key or retention hint not handled safely")
				}
				m.textInput.SetValue(tt.input)
				m.cancelled = tt.cancelled
				return m, tt.failure
			}}
			got, err := s.promptAPIKey(t.Context(), "Provider", current)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error presence = %v, want %v", err != nil, tt.wantErr)
			}
			if got != tt.want {
				t.Fatal("unexpected API key result")
			}
			if err != nil && (strings.Contains(err.Error(), current) || strings.Contains(err.Error(), replacement)) {
				t.Fatal("diagnostic disclosed API key")
			}
			var diagnostics bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelDebug}))
			logger.Debug("setup input", "error", err)
			if strings.Contains(diagnostics.String(), current) || strings.Contains(diagnostics.String(), replacement) {
				t.Fatal("structured diagnostic disclosed API key")
			}
			if tt.failure != nil && !errors.Is(err, tt.failure) {
				t.Fatal("UI error identity lost")
			}
		})
	}
}

func TestProvidersUseSecretInput(t *testing.T) {
	t.Parallel()
	setups := []struct {
		name string
		run  func(*Service, context.Context, *config.Config) error
	}{
		{"OpenAI", (*Service).setupOpenAI},
		{"Gemini", (*Service).setupGemini},
		{"Anthropic", (*Service).setupAnthropic},
		{"OpenCode", (*Service).setupOpenCode},
	}
	for _, setup := range setups {
		t.Run(setup.name, func(t *testing.T) {
			t.Parallel()
			called := false
			s := &Service{runSecretInput: func(model tea.Model) (tea.Model, error) {
				called = true
				m, ok := model.(apiKeyInput)
				if !ok {
					t.Fatalf("unexpected model type %T", model)
				}
				if !strings.Contains(m.View(), setup.name) {
					t.Fatal("missing provider prompt")
				}
				m.cancelled = true
				return m, nil
			}}
			cfg := config.DefaultConfig()
			if err := setup.run(s, t.Context(), cfg); err == nil {
				t.Fatal("cancellation ignored")
			}
			if !called {
				t.Fatal("provider did not use masked input")
			}
		})
	}
}

func TestAPIKeyTerminalOutput(t *testing.T) {
	t.Parallel()
	const current = "previous-private-credential"
	const secret = "entered-private-credential"
	var output bytes.Buffer
	s := &Service{runSecretInput: func(model tea.Model) (tea.Model, error) {
		return tea.NewProgram(model,
			tea.WithInput(strings.NewReader(secret+"\r")),
			tea.WithOutput(&output),
			tea.WithoutSignalHandler(),
		).Run()
	}}
	got, err := s.promptAPIKey(t.Context(), "Provider", current)
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if got != secret {
		t.Fatal("submitted API key changed")
	}
	if output.Len() == 0 {
		t.Fatal("terminal output was not exercised")
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), current) {
		t.Fatal("terminal output disclosed API key")
	}
}

func TestAPIKeyInputMasksEveryState(t *testing.T) {
	t.Parallel()

	const secret = "typed-private-credential"
	m := newAPIKeyInput("Enter API key:")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(secret)})
	m, ok := updated.(apiKeyInput)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if strings.Contains(m.View(), secret) || !strings.Contains(m.View(), "***") {
		t.Fatal("typed API key was not masked")
	}
	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyEsc, tea.KeyCtrlC} {
		updated, cmd := m.Update(tea.KeyMsg{Type: key})
		final, ok := updated.(apiKeyInput)
		if !ok {
			t.Fatalf("unexpected final model type %T", updated)
		}
		if strings.Contains(final.View(), secret) {
			t.Fatal("final view disclosed API key")
		}
		if cmd == nil || cmd() != tea.Quit() {
			t.Fatal("submit or cancellation did not quit")
		}
		if final.cancelled != (key != tea.KeyEnter) {
			t.Fatal("unexpected cancellation state")
		}
		if key == tea.KeyEnter && final.textInput.Value() != secret {
			t.Fatal("submitted API key changed")
		}
	}
}
