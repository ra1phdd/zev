package domain

import (
	"testing"
)

func TestCommand(t *testing.T) {
	t.Parallel()
	cmd := Command{
		Command:              "ls -la",
		ShortExplanation:     "List all files",
		IsDangerous:          false,
		DangerousExplanation: "",
	}

	if cmd.Command != "ls -la" {
		t.Errorf("expected command='ls -la', got %s", cmd.Command)
	}
	if cmd.IsDangerous {
		t.Error("expected is_dangerous=false")
	}
}

func TestOptionsResponse(t *testing.T) {
	t.Parallel()
	resp := OptionsResponse{
		Commands: []Command{
			{
				Command:          "echo hello",
				ShortExplanation: "Print hello",
				IsDangerous:      false,
			},
		},
		IsValid:               true,
		ExplanationIfNotValid: "",
	}

	if !resp.IsValid {
		t.Error("expected is_valid=true")
	}
	if len(resp.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(resp.Commands))
	}
}

func TestDangerousCommand(t *testing.T) {
	t.Parallel()
	cmd := Command{
		Command:              "rm -rf /",
		ShortExplanation:     "Remove everything",
		IsDangerous:          true,
		DangerousExplanation: "This will delete all files on your system",
	}

	if !cmd.IsDangerous {
		t.Error("expected is_dangerous=true")
	}
	if cmd.DangerousExplanation == "" {
		t.Error("expected dangerous_explanation to be set")
	}
}
