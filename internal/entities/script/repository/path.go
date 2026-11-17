package repository

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func validScriptFile(name string) error {
	if !filepath.IsLocal(name) || strings.ContainsAny(name, `\:`) {
		return fmt.Errorf("invalid script file %q", name)
	}
	for part := range strings.SplitSeq(filepath.ToSlash(name), "/") {
		if part == "." || part == ".." || part == "" {
			return fmt.Errorf("invalid script file %q", name)
		}
	}
	return nil
}

func closeResource(result *error, resource io.Closer) {
	if err := resource.Close(); err != nil {
		*result = errors.Join(*result, fmt.Errorf("close script resource: %w", err))
	}
}

func (st *store) validate(name string) error {
	if err := validScriptFile(name); err != nil {
		return err
	}
	if st.files == nil {
		return nil
	}
	return validateRelativePath(st.files, strings.Split(name, string(filepath.Separator)))
}

func validateRelativePath(root *os.Root, parts []string) (err error) {
	if len(parts) == 1 {
		return checkRegular(root, parts[0])
	}
	next, err := openDirectory(root, parts[0])
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer closeResource(&err, next)
	return validateRelativePath(next, parts[1:])
}

func checkRegular(root *os.Root, name string) error {
	if root == nil {
		return nil
	}
	f, err := openRegular(root, name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close validated file %q: %w", name, err)
	}
	return nil
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	return openRegularMode(root, name, os.O_RDONLY)
}

func openRegularMode(root *os.Root, name string, mode int) (file *os.File, err error) {
	if root == nil {
		return nil, os.ErrNotExist
	}
	// Reject the link itself before opening it. Root.OpenFile confines traversal,
	// but its platform implementation may still follow a leaf symlink.
	if info, err := root.Lstat(name); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect file %q: %w", name, err)
		}
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("file %q is a symlink", name)
	} else if !info.Mode().IsRegular() || multipleLinks(info) {
		return nil, fmt.Errorf("file %q must be regular with one link", name)
	}
	defer func() {
		if err != nil && file != nil {
			closeResource(&err, file)
			file = nil
		}
	}()
	parts := strings.Split(name, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		next, openErr := openDirectory(root, part)
		if openErr != nil {
			return nil, openErr
		}
		defer closeResource(&err, next)
		root = next
	}
	file, err = root.OpenFile(parts[len(parts)-1], mode|noFollowFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("open regular file %q: %w", name, err)
	}
	return file, validateOpenedFile(file, name)
}

func validateOpenedFile(file *os.File, name string) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat opened file %q: %w", name, err)
	}
	if err := checkOpenedFile(file); err != nil {
		return err
	}
	if !info.Mode().IsRegular() || multipleLinks(info) {
		return fmt.Errorf("file %q must be regular with one link", name)
	}
	return nil
}

func openDirectory(parent *os.Root, name string) (root *os.Root, err error) {
	if info, err := parent.Lstat(name); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect directory %q: %w", name, err)
		}
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("directory %q is a symlink", name)
	}
	f, err := parent.OpenFile(name, os.O_RDONLY|directoryFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("pin directory %q: %w", name, err)
	}
	defer func() {
		closeResource(&err, f)
		if err != nil && root != nil {
			closeResource(&err, root)
			root = nil
		}
	}()
	before, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat directory %q: %w", name, err)
	}
	if !before.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", name)
	}
	root, err = parent.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open directory %q: %w", name, err)
	}
	after, err := root.Stat(".")
	if err != nil {
		return root, fmt.Errorf("stat opened directory %q: %w", name, err)
	}
	if !os.SameFile(before, after) {
		return root, fmt.Errorf("directory %q changed while opening", name)
	}
	return root, nil
}

// validateFilePath permits missing paths but rejects links in application-owned
// components. Repository construction has already resolved the trusted parent.
func validateFilePath(path string, directory bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", path, err)
	}
	current := filepath.VolumeName(abs) + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect path component %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path component %q is not allowed", current)
		}
		if i < len(parts)-1 || directory {
			if !info.IsDir() {
				return fmt.Errorf("%q is not a directory", current)
			}
		} else if !info.Mode().IsRegular() || multipleLinks(info) {
			return fmt.Errorf("%q must be regular with one link", current)
		}
	}
	return nil
}
