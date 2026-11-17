// Package contextfile loads bounded, non-sensitive project context files.
package contextfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxFileBytes limits one context file to 64 KiB.
	MaxFileBytes = 64 * 1024
	// MaxTotalBytes limits all context files in one request to 256 KiB.
	MaxTotalBytes = 256 * 1024
)

var (
	// ErrFileTooLarge reports a per-file size violation.
	ErrFileTooLarge = errors.New("context file exceeds the per-file limit")
	// ErrTotalTooLarge reports a total request size violation.
	ErrTotalTooLarge = errors.New("context files exceed the total limit")
	// ErrSensitiveFile reports a protected filename.
	ErrSensitiveFile = errors.New("sensitive context files are not allowed")
	// ErrSymlink reports a rejected symbolic link.
	ErrSymlink = errors.New("context symlinks are not allowed")
	// ErrOutsideRoot reports a path outside the allowed root.
	ErrOutsideRoot = errors.New("context file is outside the allowed context root")
)

// Load reads paths relative to root and returns a sectioned PROJECT_CONTEXT.
// Only regular, non-sensitive files below root are accepted. Symlinks are
// rejected before opening, so a context file cannot redirect the model read.
func Load(root string, paths []string) (string, error) {
	return LoadWithOptions(root, paths, false)
}

// LoadWithOptions is like Load and permits sensitive filenames only when explicitly requested.
func LoadWithOptions(root string, paths []string, allowSensitive bool) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}
	allowedRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve context root: %w", err)
	}
	var total int
	var sections strings.Builder
	sections.WriteString("[PROJECT_CONTEXT]\n")
	for _, path := range paths {
		data, display, err := loadOne(allowedRoot, path, total, allowSensitive)
		if err != nil {
			return "", err
		}
		total += len(data)
		fmt.Fprintf(&sections, "--- FILE: %s ---\n%s\n--- END FILE: %s ---\n", display, data, display)
	}
	sections.WriteString("[/PROJECT_CONTEXT]")
	return sections.String(), nil
}

func loadOne(root, path string, total int, allowSensitive bool) ([]byte, string, error) {
	absolute, err := filepath.Abs(path)
	if !filepath.IsAbs(path) {
		absolute, err = filepath.Abs(filepath.Join(root, path))
	}
	if err != nil {
		return nil, "", fmt.Errorf("resolve context file %q: %w", path, err)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrOutsideRoot)
	}
	if err := rejectSymlinkComponents(root, absolute); err != nil {
		return nil, "", fmt.Errorf("context file %q: %w", path, err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, "", fmt.Errorf("inspect context file %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrSymlink)
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("context file %q is not a regular file", path)
	}
	if isSensitive(filepath.Base(absolute)) && !allowSensitive {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrSensitiveFile)
	}
	if info.Size() > MaxFileBytes {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrFileTooLarge)
	}
	if total > MaxTotalBytes-int(info.Size()) {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrTotalTooLarge)
	}
	file, err := os.Open(absolute) //nolint:gosec // Path is confined and validated above.
	if err != nil {
		return nil, "", fmt.Errorf("open context file %q: %w", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			return
		}
	}()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return nil, "", fmt.Errorf("context file %q is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read context file %q: %w", path, err)
	}
	if len(data) > MaxFileBytes {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrFileTooLarge)
	}
	return data, filepath.ToSlash(relative), nil
}

func rejectSymlinkComponents(root, absolute string) error {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return fmt.Errorf("resolve context components: %w", err)
	}
	current := root
	for component := range strings.SplitSeq(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("inspect context component: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrSymlink
		}
	}
	return nil
}

func isSensitive(name string) bool {
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}
	for _, marker := range []string{"credential", "secret", "password", "token"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if strings.HasPrefix(lower, "id_") || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx") {
		return true
	}
	return false
}
