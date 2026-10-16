package terminal

import (
	"context"
	"os/exec"
	"testing"
)

func TestExecute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "successful command",
			command: "echo 'test'",
			wantErr: false,
		},
		{
			name:    "command with exit code 0",
			command: "true",
			wantErr: false,
		},
		{
			name:    "command with exit code 1",
			command: "false",
			wantErr: true,
		},
		{
			name:    "non-existent command",
			command: "nonexistentcommand12345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Execute(context.Background(), tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Для команд с ненулевым кодом проверяем тип ошибки
			if tt.wantErr && err != nil {
				exitError := &exec.ExitError{}
				if errors.As(err, &exitError) {
					t.Errorf("Execute() error type = %T, want *exec.ExitError", err)
				}
			}
		})
	}
}
