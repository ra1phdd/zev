package domain

import (
	"encoding/json"
	"errors"
	"strings"
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

func TestOptionsResponseCompactJSONEscapesCommand(t *testing.T) {
	t.Parallel()

	command := "printf \"quoted\\\\path\"\n"
	response := &OptionsResponse{
		Commands: []Command{{Command: command}},
		IsValid:  true,
	}

	var compact struct {
		Commands []string `json:"commands"`
	}
	if err := json.Unmarshal([]byte(response.CompactJSON()), &compact); err != nil {
		t.Fatalf("decode compact JSON: %v", err)
	}
	if len(compact.Commands) != 1 || compact.Commands[0] != command {
		t.Fatalf("compact commands = %#v, want %#v", compact.Commands, []string{command})
	}
}

func TestOptionsResponseValidate(t *testing.T) {
	t.Parallel()

	valid := func() OptionsResponse {
		return OptionsResponse{
			Commands: []Command{{
				Command:          "printf hello",
				ShortExplanation: "Print hello",
			}},
			IsValid: true,
		}
	}

	tests := []struct {
		name    string
		prepare func(*OptionsResponse)
	}{
		{
			name: "nil response",
		},
		{
			name: "no commands",
			prepare: func(response *OptionsResponse) {
				response.Commands = nil
			},
		},
		{
			name: "empty command",
			prepare: func(response *OptionsResponse) {
				response.Commands[0].Command = ""
			},
		},
		{
			name: "blank command",
			prepare: func(response *OptionsResponse) {
				response.Commands[0].Command = "   "
			},
		},
		{
			name: "too many commands",
			prepare: func(response *OptionsResponse) {
				response.Commands = []Command{
					{Command: "one"}, {Command: "two"}, {Command: "three"}, {Command: "four"},
				}
			},
		},
		{
			name: "control character in command",
			prepare: func(response *OptionsResponse) {
				response.Commands[0].Command = "printf hello\x00world"
			},
		},
		{
			name: "invalid UTF-8 command",
			prepare: func(response *OptionsResponse) {
				response.Commands[0].Command = string([]byte{0xff})
			},
		},
		{
			name: "command too long",
			prepare: func(response *OptionsResponse) {
				response.Commands[0].Command = strings.Repeat("x", MaxCommandLength+1)
			},
		},
		{
			name: "explanation too long",
			prepare: func(response *OptionsResponse) {
				response.Commands[0].ShortExplanation = strings.Repeat("x", MaxExplanationLength+1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := valid()
			if test.prepare == nil {
				var nilResponse *OptionsResponse
				if err := nilResponse.Validate(); err == nil {
					t.Fatal("Validate() error = nil, want error")
				} else if !errors.Is(err, ErrInvalidOptionsResponse) {
					t.Fatalf("Validate() error = %v, want ErrInvalidOptionsResponse", err)
				}
				return
			}
			test.prepare(&response)
			if err := response.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want error")
			} else if !errors.Is(err, ErrInvalidOptionsResponse) {
				t.Fatalf("Validate() error = %v, want ErrInvalidOptionsResponse", err)
			}
		})
	}
}

func TestOptionsResponseValidateAcceptsAllowedWhitespace(t *testing.T) {
	t.Parallel()

	response := OptionsResponse{
		Commands: []Command{{Command: "printf hello\n	world\r"}},
		IsValid:  true,
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestOptionsResponseValidateAcceptsValidResponse(t *testing.T) {
	t.Parallel()

	response := OptionsResponse{
		Commands: []Command{{
			Command:              `printf "quoted\\path"`,
			ShortExplanation:     "Print a quoted path",
			IsDangerous:          false,
			DangerousExplanation: "",
		}},
		IsValid: true,
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
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
