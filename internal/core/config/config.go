// Package config loads and saves Zev settings from the environment and config file.
package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/ra1phdd/zev/internal/core/filestore"
)

// Config contains provider, HTTP, and command execution settings.
type Config struct {
	UILanguage  string            `env:"ZEV_LANG"`
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
	HistoryEnabled       bool   `env:"HISTORY_ENABLED"`
	ContextEnabled       bool   `env:"CONTEXT_ENABLED"`
	ToolsEnabled         bool   `env:"TOOLS_ENABLED"`
	ToolApprovalMode     string `env:"TOOL_APPROVAL_MODE"`

	HistorySemanticEnabled     bool   `env:"HISTORY_SEMANTIC_ENABLED"`
	HistoryEmbeddingProvider   string `env:"HISTORY_EMBEDDING_PROVIDER"`
	HistoryEmbeddingBaseURL    string `env:"HISTORY_EMBEDDING_BASE_URL"`
	HistoryEmbeddingModel      string `env:"HISTORY_EMBEDDING_MODEL"`
	HistoryEmbeddingDimensions int    `env:"HISTORY_EMBEDDING_DIMENSIONS"`
	HistoryEmbeddingAPIKey     string `env:"HISTORY_EMBEDDING_API_KEY"`
	HistoryEmbeddingMaxBytes   int    `env:"HISTORY_EMBEDDING_MAX_BYTES"`
}

// DefaultConfig returns a new configuration with default provider and CLI settings.
func DefaultConfig() *Config {
	return &Config{
		UILanguage:  uiLanguageAuto,
		LLMProvider: "openai",
		HTTPHeaders: map[string]string{
			"User-Agent": "zev/1.0",
		},
		OpenAIModel:              "gpt-5.6-luna",
		OpenAIBaseURL:            "https://api.openai.com/v1",
		GeminiModel:              "gemini-3.1-flash-lite",
		GeminiBaseURL:            "https://generativelanguage.googleapis.com",
		AnthropicModel:           "claude-haiku-5.5",
		AnthropicBaseURL:         "https://api.anthropic.com/v1",
		CommandExecutionMode:     "inject",
		ScriptEditor:             "nano",
		HistoryEnabled:           true,
		ContextEnabled:           true,
		ToolsEnabled:             true,
		ToolApprovalMode:         "manual",
		HistoryEmbeddingMaxBytes: 32768,
	}
}

// Load reads settings with environment values taking precedence over the config
// file and defaults. Symlinks at the config file or application directory are
// rejected. It returns ErrInvalidHome before filesystem I/O if the platform home
// is missing or relative, or an error if loading or environment parsing fails.
func Load() (cfg *Config, err error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}
	if err := filestore.ValidateDirectory(filepath.Dir(configPath)); err != nil {
		return nil, fmt.Errorf("validate config directory: %w", err)
	}
	err = filestore.WithLock(configPath, func() error {
		cfg, err = load(configPath)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load config transaction: %w", err)
	}
	return cfg, nil
}

func load(configPath string) (*Config, error) {
	values, err := readValues(configPath)
	if err != nil {
		return nil, err
	}
	environment := env.ToMap(os.Environ())
	if strings.TrimSpace(environment["ZEV_LANG"]) == "" {
		delete(environment, "ZEV_LANG")
	}
	maps.Copy(values, environment)
	return parseValues(values)
}

// configError keeps parser causes available for classification without printing
// provider keys or malformed file contents in diagnostics.
type configError struct {
	message string
	cause   error
}

func (e configError) Error() string { return e.message }
func (e configError) Unwrap() error { return e.cause }

func readValues(path string) (map[string]string, error) {
	data, err := filestore.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, configError{message: "failed to read config file", cause: err}
	}
	values, err := godotenv.UnmarshalBytes(data)
	if err != nil {
		return nil, configError{message: "failed to read config file", cause: err}
	}
	return values, nil
}

var (
	errInvalidToolPolicy = errors.New("invalid tool policy")
	errInvalidUILanguage = errors.New("invalid UI language: must be auto, en, or ru")
)

func validationConfigError(err error) configError {
	message := "invalid history embedding settings"
	if errors.Is(err, errInvalidToolPolicy) {
		message = "invalid tool policy"
	}
	if errors.Is(err, errInvalidUILanguage) {
		message = "invalid UI language: must be auto, en, or ru"
	}
	return configError{message: message, cause: err}
}

func parseValues(values map[string]string) (*Config, error) {
	cfg := DefaultConfig()
	if err := env.ParseWithOptions(cfg, env.Options{Environment: values}); err != nil {
		return nil, configError{message: "failed to parse config settings", cause: err}
	}
	cfg.UILanguage = normalizeUILanguage(cfg.UILanguage)
	if headers, ok := values["HTTP_HEADERS"]; ok {
		cfg.HTTPHeaders = parseHTTPHeaders(headers)
	}
	if mode, ok := values["TOOL_APPROVAL_MODE"]; ok && strings.TrimSpace(mode) == "" {
		return nil, validationConfigError(fmt.Errorf("%w: approval mode must be manual, auto, or bypass", errInvalidToolPolicy))
	}
	if err := cfg.Validate(); err != nil {
		return nil, validationConfigError(err)
	}
	return cfg, nil
}

// Validate checks UI language, tool policy, and opt-in history embedding settings.
// Empty UI language means auto. Disabled semantic search deliberately ignores the
// other embedding fields. Invalid UI language errors never include its value.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	c.UILanguage = normalizeUILanguage(c.UILanguage)
	if err := validateUILanguage(c.UILanguage); err != nil {
		return err
	}
	if c.ToolApprovalMode != "manual" && c.ToolApprovalMode != "auto" && c.ToolApprovalMode != "bypass" {
		return fmt.Errorf("%w: approval mode must be manual, auto, or bypass", errInvalidToolPolicy)
	}
	if !c.HistorySemanticEnabled {
		return nil
	}
	if c.HistoryEmbeddingProvider != "openai-compatible" {
		return errors.New("unsupported history embedding provider")
	}
	if strings.TrimSpace(c.HistoryEmbeddingBaseURL) == "" {
		return errors.New("history embedding base URL is required")
	}
	if strings.TrimSpace(c.HistoryEmbeddingModel) == "" {
		return errors.New("history embedding model is required")
	}
	if c.HistoryEmbeddingDimensions <= 0 {
		return errors.New("history embedding dimensions must be positive")
	}
	if c.HistoryEmbeddingMaxBytes <= 0 {
		return errors.New("history embedding max bytes must be positive")
	}
	return nil
}

// Save atomically replaces the config file with the nonzero fields of cfg.
// Use Update for read-modify-write operations. Failures before rename leave an
// existing configuration unchanged; errors.Is(err, filestore.ErrCommitted) marks
// a visible replacement with uncertain crash durability. cfg must not be nil.
// ErrInvalidHome is returned before filesystem I/O for a missing or relative home.
func Save(cfg *Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	if err := filestore.ValidateDirectory(filepath.Dir(configPath)); err != nil {
		return fmt.Errorf("validate config directory: %w", err)
	}
	if err := filestore.WithLock(configPath, func() error {
		return save(configPath, cfg)
	}); err != nil {
		return fmt.Errorf("save transaction: %w", err)
	}
	return nil
}

// Update changes the persisted configuration under one cooperative transaction
// lock. It loads file values over defaults, deliberately excluding environment
// overrides so that ambient settings do not overwrite unrelated persisted values.
// If change fails or the existing file is malformed, no replacement is written.
// The callback must not call Load, Save, or Update for the same configuration.
// Write errors preserve filestore.ErrCommitted after a successful replacement.
// ErrInvalidHome is returned before filesystem I/O for a missing or relative home.
func Update(change func(*Config) error) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	if err := filestore.ValidateDirectory(filepath.Dir(configPath)); err != nil {
		return fmt.Errorf("validate config directory: %w", err)
	}
	if err := filestore.WithLock(configPath, func() error {
		values, err := readValues(configPath)
		if err != nil {
			return err
		}
		cfg, err := parseValues(values)
		if err != nil {
			return err
		}
		if err := change(cfg); err != nil {
			return err
		}
		return save(configPath, cfg)
	}); err != nil {
		return fmt.Errorf("update config: %w", err)
	}
	return nil
}

func save(configPath string, cfg *Config) error {
	if cfg == nil {
		return errors.New("config must not be nil")
	}
	normalized := *cfg
	normalized.UILanguage = normalizeUILanguage(cfg.UILanguage)
	cfg = &normalized
	if err := cfg.Validate(); err != nil {
		return validationConfigError(err)
	}
	values := make(map[string]string)
	v := reflect.ValueOf(*cfg)
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		value := v.Field(i)
		if value.IsZero() && value.Kind() != reflect.Bool {
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
				values[envTag] = strings.Join(headerPairs, ",")
			}
			continue
		}

		values[envTag] = fmt.Sprint(value.Interface())
	}

	content, err := godotenv.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := filestore.AtomicWrite(configPath, []byte(content+"\n"), 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

var configPathForTests string

func getConfigPath() (string, error) {
	if configPathForTests != "" {
		if !filepath.IsAbs(configPathForTests) {
			return "", errors.New("test config path must be absolute")
		}
		return configPathForTests, nil
	}
	dir, err := GetZevDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config"), nil
}

// ErrInvalidHome indicates that the platform home environment variable is missing
// or not absolute. Callers can classify resolution failures with errors.Is.
var ErrInvalidHome = errors.New("home directory must be set to an absolute path")

// GetZevDir returns the absolute path to ~/.zev without filesystem I/O.
// It returns an empty path and ErrInvalidHome if HOME (USERPROFILE on Windows,
// home on Plan 9) is missing or relative; it never falls back to the working directory.
// Trusted aliases in absolute home paths are preserved for filesystem validation.
func GetZevDir() (string, error) {
	homeEnv := "HOME"
	switch runtime.GOOS {
	case "windows":
		homeEnv = "USERPROFILE"
	case "plan9":
		homeEnv = "home"
	}
	home := os.Getenv(homeEnv)
	if !filepath.IsAbs(home) {
		return "", ErrInvalidHome
	}
	return filepath.Join(home, ".zev"), nil
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
