package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	LLMProvider string            `env:"LLM_PROVIDER"`
	HTTPHeaders map[string]string `env:"HTTP_HEADERS" envSeparator:","`

	OpenAIAPIKey  string `env:"OPENAI_API_KEY"`
	OpenAIModel   string `env:"OPENAI_MODEL"`
	OpenAIBaseURL string `env:"OPENAI_BASE_URL"`
	GeminiAPIKey  string `env:"GEMINI_API_KEY"`
	GeminiModel   string `env:"GEMINI_MODEL"`
	GeminiBaseURL string `env:"GEMINI_BASE_URL"`

	AnthropicAPIKey  string `env:"ANTHROPIC_API_KEY"`
	AnthropicModel   string `env:"ANTHROPIC_MODEL"`
	AnthropicBaseURL string `env:"ANTHROPIC_BASE_URL"`

	OllamaBaseURL string `env:"OLLAMA_BASE_URL"`
	OllamaModel   string `env:"OLLAMA_MODEL"`

	CommandExecutionMode string `env:"COMMAND_EXECUTION_MODE"`
	ScriptEditor         string `env:"SCRIPT_EDITOR"`
}

func DefaultConfig() *Config {
	return &Config{
		LLMProvider: "openai",
		HTTPHeaders: map[string]string{
			"User-Agent": "zev/1.0",
		},
		OpenAIModel:          "gpt-5.6-luna",
		OpenAIBaseURL:        "https://api.openai.com/v1",
		GeminiModel:          "gemini-3.1-flash-lite",
		GeminiBaseURL:        "https://generativelanguage.googleapis.com",
		AnthropicModel:       "claude-haiku-5.5",
		AnthropicBaseURL:     "https://api.anthropic.com/v1",
		CommandExecutionMode: "inject",
		ScriptEditor:         "nano",
	}
}

func Load() (*Config, error) {
	cfg := DefaultConfig()

	configPath := getConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		if err := godotenv.Load(configPath); err != nil {
			return nil, fmt.Errorf("failed to load .env: %w", err)
		}
	}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse env: %w", err)
	}

	if headersEnv := os.Getenv("HTTP_HEADERS"); headersEnv != "" {
		cfg.HTTPHeaders = parseHTTPHeaders(headersEnv)
	}

	return cfg, nil
}

func Save(cfg *Config) error {
	configPath := getConfigPath()

	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	var lines []string
	v := reflect.ValueOf(*cfg)
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		value := v.Field(i)
		if value.IsZero() {
			continue
		}

		if value.Kind() == reflect.Map && value.Type().Key().Kind() == reflect.String && value.Type().Elem().Kind() == reflect.String {
			var headerPairs []string
			iter := value.MapRange()
			for iter.Next() {
				k := iter.Key().String()
				v := iter.Value().String()
				headerPairs = append(headerPairs, fmt.Sprintf("%s:%s", k, v))
			}
			if len(headerPairs) > 0 {
				lines = append(lines, fmt.Sprintf("%s=%s", envTag, strings.Join(headerPairs, ",")))
			}
			continue
		}

		lines = append(lines, fmt.Sprintf("%s=%v", envTag, value.Interface()))
	}

	content := strings.Join(lines, "\n") + "\n"

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

var getConfigPath = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zev", "config")
}

// SetConfigPathForTests заменяет getConfigPath для тестов и возвращает старое значение.
func SetConfigPathForTests(newPath string) string {
	old := getConfigPath()
	getConfigPath = func() string {
		return newPath
	}
	return old
}

// GetZevDir возвращает путь к директории ~/.zev.
var GetZevDir = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zev")
}

func parseHTTPHeaders(input string) map[string]string {
	if input == "" {
		return nil
	}

	headers := make(map[string]string)
	pairs := strings.SplitSeq(input, ",")

	for pair := range pairs {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				headers[key] = value
			}
		}
	}

	return headers
}
