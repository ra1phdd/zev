package domain

import (
	"encoding/json"
	"testing"
)

func TestOptionsResponseJSONContract(t *testing.T) {
	t.Parallel()

	const fixture = `{"commands":[{"command":"rm file","short_explanation":"Remove file","is_dangerous":true,"dangerous_explanation":"Deletes file"}],"is_valid":false,"explanation_if_not_valid":"Needs clarification"}`
	var response OptionsResponse
	if err := json.Unmarshal([]byte(fixture), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Commands) != 1 {
		t.Fatalf("expected one command, got %d", len(response.Commands))
	}
	command := response.Commands[0]
	if command.Command != "rm file" || command.ShortExplanation != "Remove file" || !command.IsDangerous || command.DangerousExplanation != "Deletes file" {
		t.Errorf("unexpected decoded command: %+v", command)
	}
	if response.IsValid || response.ExplanationIfNotValid != "Needs clarification" {
		t.Errorf("unexpected decoded response: %+v", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	if string(encoded) != fixture {
		t.Errorf("encoded response = %s, want %s", encoded, fixture)
	}
}

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
