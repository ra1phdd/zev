package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/zev-go/internal/core/config"
)

//nolint:paralleltest // меняет глобальную config.getConfigPath
func TestSetupMode(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		wantOut string
	}{
		{
			name:    "valid inject mode",
			args:    []string{"inject"},
			wantErr: false,
			wantOut: "",
		},
		{
			name:    "valid execute mode",
			args:    []string{"execute"},
			wantErr: false,
			wantOut: "",
		},
		{
			name:    "invalid mode",
			args:    []string{"invalid"},
			wantErr: true,
			wantOut: "",
		},
		{
			name:    "show current mode",
			args:    []string{},
			wantErr: false,
			wantOut: "Current command execution mode: inject\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Очищаем переменные окружения перед каждым тестом
			os.Unsetenv("COMMAND_EXECUTION_MODE")

			// Создаём временную директорию для конфига
			tmpDir := t.TempDir()
			tmpConfigPath := filepath.Join(tmpDir, "config")

			// Подменяем config.getConfigPath на время теста
			originalGetConfigPath := config.SetConfigPathForTests(tmpConfigPath)
			t.Cleanup(func() {
				config.SetConfigPathForTests(originalGetConfigPath)
			})

			// Создаём дефолтный конфиг
			cfg := config.DefaultConfig()
			if err := config.Save(cfg); err != nil {
				t.Fatalf("failed to save default config: %v", err)
			}

			// Перенаправляем stdout если нужно проверить вывод
			var oldStdout *os.File
			var r *os.File
			if tt.wantOut != "" {
				oldStdout = os.Stdout
				var w *os.File
				r, w, _ = os.Pipe()
				os.Stdout = w
			}

			// Создаём команду и запускаем
			cmd := newSetupModeCmd()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			// Восстанавливаем stdout и читаем вывод
			if tt.wantOut != "" {
				os.Stdout.Close()
				os.Stdout = oldStdout
				buf := make([]byte, 1024)
				n, _ := r.Read(buf)
				output := string(buf[:n])
				if output != tt.wantOut {
					t.Errorf("unexpected output: got %q, want %q", output, tt.wantOut)
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("runSetupMode() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Если не должно быть ошибки и это не показ текущего режима, проверяем сохранённое значение
			if !tt.wantErr && len(tt.args) > 0 {
				// Очищаем переменные окружения чтобы Load() читал из файла
				os.Unsetenv("COMMAND_EXECUTION_MODE")

				loadedCfg, err := config.Load()
				if err != nil {
					t.Fatalf("failed to load config after save: %v", err)
				}
				if loadedCfg.CommandExecutionMode != tt.args[0] {
					t.Errorf("CommandExecutionMode = %v, want %v", loadedCfg.CommandExecutionMode, tt.args[0])
				}
			}
		})
	}
}
