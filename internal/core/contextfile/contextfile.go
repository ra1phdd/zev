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
	// ErrSensitiveFile reports a protected filename or path component.
	ErrSensitiveFile = errors.New("sensitive context files are not allowed")
	// ErrSymlink reports a rejected symbolic link.
	ErrSymlink = errors.New("context symlinks are not allowed")
	// ErrOutsideRoot reports a path outside the allowed root.
	ErrOutsideRoot = errors.New("context file is outside the allowed context root")
)

// LoadWithOptions reads paths relative to root into a sectioned PROJECT_CONTEXT.
// Only regular files below root are accepted. Root is trusted configuration: its
// ancestors may contain OS aliases, but root itself and all requested components
// must not be symlinks. Opened identities are checked before reading. Sensitive
// filenames and directories require allowSensitive; that option never relaxes
// confinement or file-type checks. Sensitivity is not a content-secret detector.
// An error returns no partial context. Use errors.Is with ErrSensitiveFile,
// ErrSymlink, ErrOutsideRoot, ErrFileTooLarge, ErrTotalTooLarge, or filesystem errors.
// Byte limits do not impose a deadline on synchronous filesystem operations.
func LoadWithOptions(root string, paths []string, allowSensitive bool) (result string, err error) {
	if len(paths) == 0 {
		return "", nil
	}
	allowedRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve context root: %w", err)
	}
	pinned, err := openContextRoot(allowedRoot)
	if err != nil {
		return "", fmt.Errorf("open context root: %w", err)
	}
	defer func() {
		if closeErr := pinned.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close context root: %w", closeErr))
			result = ""
		}
	}()
	l := loader{root: pinned, rootPath: allowedRoot, allowSensitive: allowSensitive}
	var total int
	var sections strings.Builder
	sections.WriteString("[PROJECT_CONTEXT]\n")
	for _, path := range paths {
		data, display, err := l.loadOne(path, total)
		if err != nil {
			return "", err
		}
		total += len(data)
		fmt.Fprintf(&sections, "--- FILE: %s ---\n%s\n--- END FILE: %s ---\n", display, data, display)
	}
	sections.WriteString("[/PROJECT_CONTEXT]")
	return sections.String(), nil
}

func openContextRoot(path string) (*os.Root, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect context root: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlink
	}
	if !before.IsDir() {
		return nil, fmt.Errorf("context root %q is not a directory", path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("pin context root: %w", err)
	}
	after, err := root.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		return nil, errors.Join(errors.New("context root changed while opening"), err, root.Close())
	}
	return root, nil
}

type loader struct {
	root           *os.Root
	rootPath       string
	allowSensitive bool
}

func (l loader) loadOne(path string, total int) (data []byte, display string, err error) {
	absolute := path
	if !filepath.IsAbs(path) {
		absolute = filepath.Join(l.rootPath, path)
	}
	relative, err := filepath.Rel(l.rootPath, absolute)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrOutsideRoot)
	}
	if !l.allowSensitive {
		for component := range strings.SplitSeq(relative, string(filepath.Separator)) {
			if isSensitive(component) {
				return nil, "", fmt.Errorf("context file %q: %w", path, ErrSensitiveFile)
			}
		}
	}
	file, err := openContextFile(l.root, relative)
	if err != nil {
		return nil, "", fmt.Errorf("context file %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close context file %q: %w", path, closeErr))
			data = nil
		}
	}()
	opened, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("inspect context file %q: %w", path, err)
	}
	if opened.Size() > MaxFileBytes {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrFileTooLarge)
	}
	if total > MaxTotalBytes-int(opened.Size()) {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrTotalTooLarge)
	}
	limit := min(MaxFileBytes, MaxTotalBytes-total)
	data, err = io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, "", fmt.Errorf("read context file %q: %w", path, err)
	}
	if len(data) > MaxFileBytes {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrFileTooLarge)
	}
	if len(data) > MaxTotalBytes-total {
		return nil, "", fmt.Errorf("context file %q: %w", path, ErrTotalTooLarge)
	}
	return data, filepath.ToSlash(relative), nil
}

func openContextFile(root *os.Root, path string) (*os.File, error) {
	component, remaining, nested := strings.Cut(path, string(filepath.Separator))
	if nested {
		return openContextDescendant(root, component, remaining)
	}
	before, err := root.Lstat(component)
	if err != nil {
		return nil, fmt.Errorf("inspect context component: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlink
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("context file %q is not a regular file", path)
	}
	file, err := openContextLeaf(root, component)
	if err != nil {
		return nil, fmt.Errorf("open context file: %w", err)
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, errors.Join(errors.New("context file changed while opening or is not a regular file"), err, file.Close())
	}
	return file, nil
}

func openContextDescendant(root *os.Root, component, remaining string) (*os.File, error) {
	before, err := root.Lstat(component)
	if err != nil {
		return nil, fmt.Errorf("inspect context component: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlink
	}
	if !before.IsDir() {
		return nil, fmt.Errorf("context component %q is not a directory", component)
	}
	child, err := root.OpenRoot(component)
	if err != nil {
		return nil, fmt.Errorf("open context directory: %w", err)
	}
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		return nil, errors.Join(errors.New("context directory changed while opening"), err, child.Close())
	}
	file, openErr := openContextFile(child, remaining)
	if closeErr := child.Close(); closeErr != nil {
		if file != nil {
			closeErr = errors.Join(closeErr, file.Close())
		}
		return nil, errors.Join(openErr, fmt.Errorf("close context directory: %w", closeErr))
	}
	return file, openErr
}

func isSensitive(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case ".aws", ".docker", ".ssh", ".kube", ".azure", ".gnupg", ".zev":
		return true
	}
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}
	if strings.HasSuffix(lower, ".go") {
		return false
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
