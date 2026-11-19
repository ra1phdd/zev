package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ra1phdd/zev/internal/core/filestore"
)

const configFilename = "config.yaml"

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
	return filepath.Join(dir, configFilename), nil
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

func transaction(ctx context.Context, op func(string) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("configuration transaction: %w", err)
	}
	path, err := getConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := filestore.ValidateDirectory(dir); err != nil {
		return fmt.Errorf("validate config directory: %w", err)
	}
	if err := filestore.WithLockContext(ctx, path, func() error { return op(path) }); err != nil {
		return fmt.Errorf("config transaction: %w", err)
	}
	return nil
}

func load(path string) (*Config, error) {
	if err := ValidateFileSecurity(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("validate configuration security: %w", err)
	}
	data, err := filestore.ReadSingleLink(path, maxConfigBytes)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	return Parse(data)
}

// Load reads the current config.yaml. An absent file
// yields defaults. No configuration values come from the process environment.
func Load() (*Config, error) {
	return LoadContext(context.Background())
}

// LoadContext reads the current config.yaml with cancellation-aware locks.
func LoadContext(ctx context.Context) (*Config, error) {
	var cfg *Config
	err := transaction(ctx, func(path string) error {
		var err error
		cfg, err = load(path)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}

// Save replaces a validated YAML configuration atomically. Prefer Update when
// changing an existing file. A failed directory sync can wrap filestore.ErrCommitted.
func Save(cfg *Config) error {
	return transaction(context.Background(), func(path string) error {
		if _, err := load(path); err != nil {
			return err
		}
		return writeYAML(path, cfg)
	})
}

func writeYAML(path string, cfg *Config) error {
	data, err := encodeYAML(cfg)
	if err != nil {
		return err
	}
	if err := filestore.AtomicWrite(path, data, 0o600); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	return nil
}

// Update holds the configuration lock through load, change and publication.
// The callback must not call Load, Save or Update; its error aborts the write.
func Update(change func(*Config) error) error {
	return UpdateContext(context.Background(), change)
}

// UpdateContext changes the persisted configuration under a cancellation-aware
// transaction lock. The callback runs only after the current document validates.
func UpdateContext(ctx context.Context, change func(*Config) error) error {
	return transaction(ctx, func(path string) error {
		if change == nil {
			return errors.New("config change must not be nil")
		}
		cfg, err := load(path)
		if err != nil {
			return err
		}
		if err := change(cfg); err != nil {
			return err
		}
		return writeYAML(path, cfg)
	})
}
