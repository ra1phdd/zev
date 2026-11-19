package config

import (
	"errors"
	"fmt"
	"os"
)

// ValidateFileSecurity rejects configuration files that are not private regular files owned by the current user.
func ValidateFileSecurity(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect configuration security: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("configuration must be a regular file")
	}
	return validateFileOwner(path, info)
}
