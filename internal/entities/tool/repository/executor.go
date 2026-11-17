// Package repository executes local model tool requests.
package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	maxOutput     = 32 * 1024
	maxEntries    = 1024
	maxComponents = 64
	maxPath       = 4096
	builtinPWD    = "pwd"
	builtinLS     = "ls"
	builtinCat    = "cat"
	builtinHead   = "head"
)

// ExecRequest is the domain's literal executable-and-arguments request.
type ExecRequest = domain.ExecRequest

// Result is the domain's bounded execution result.
type Result = domain.ToolResult

// ErrDenied identifies unsupported or unsafe restricted requests; use errors.Is.
var ErrDenied = errors.New("tool execution denied")

// Executor executes from a validated project directory. Its zero value denies
// execution. It is safe for concurrent use; callers must Close the pinned root.
type Executor struct {
	root     string
	identity os.FileInfo
	pinned   *os.Root
}

// New validates an existing root directory and remembers its filesystem identity.
// Relative roots are resolved against cwd. Symlinks in root or any ancestor are
// rejected, not resolved. Callers with trusted OS aliases must canonicalize first.
// Each operation reopens and verifies the root; replacing it requires a new Executor.
// Call Close when the executor is no longer needed.
func New(root string) (*Executor, error) {
	if root == "" || len(root) > maxPath {
		return nil, fmt.Errorf("invalid execution root: %w", ErrDenied)
	}
	if hasTraversal(root) {
		return nil, fmt.Errorf("execution root traversal: %w", ErrDenied)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve execution root: %w", err)
	}
	r, err := openAbsoluteRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("open execution root: %w", err)
	}
	info, err := r.Stat(".")
	if err != nil {
		return nil, fmt.Errorf("inspect execution root: %w", errors.Join(err, r.Close()))
	}
	return &Executor{root: absolute, identity: info, pinned: r}, nil
}

// Close releases the pinned root. It is safe alongside concurrent operations;
// already-open operations may finish, but new operations are denied afterward.
func (e *Executor) Close() error {
	if e == nil || e.pinned == nil {
		return nil
	}
	if err := e.pinned.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("close execution root: %w", err)
	}
	return nil
}

type builtin struct {
	path  string
	lines int
}

func parseBuiltin(request ExecRequest) (builtin, error) {
	if !validProcessRequest(request) {
		return builtin{}, ErrDenied
	}
	b := builtin{path: ".", lines: 10}
	switch request.Command {
	case builtinPWD:
		if len(request.Args) != 0 {
			return b, ErrDenied
		}
	case builtinLS:
		if len(request.Args) > 1 {
			return b, ErrDenied
		}
		if len(request.Args) == 1 {
			b.path = request.Args[0]
		}
	case builtinCat:
		if len(request.Args) != 1 {
			return b, ErrDenied
		}
		b.path = request.Args[0]
	case builtinHead:
		switch len(request.Args) {
		case 1:
			b.path = request.Args[0]
		case 3:
			if request.Args[0] != "-n" {
				return b, ErrDenied
			}
			n, err := strconv.Atoi(request.Args[1])
			if err != nil || n < 1 || n > 1000 || strconv.Itoa(n) != request.Args[1] {
				return b, ErrDenied
			}
			b.lines = n
			b.path = request.Args[2]
		default:
			return b, ErrDenied
		}
	default:
		return b, ErrDenied
	}
	return b, nil
}

// CheckReadOnly validates the exact builtin grammar, root identity, and opened
// target before a remote reviewer is consulted. Success grants no authority:
// Execute repeats all validation. Failures wrap ErrDenied.
func (e *Executor) CheckReadOnly(request ExecRequest) (err error) {
	file, _, err := e.openBuiltin(request)
	if err != nil {
		return fmt.Errorf("read-only check: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close checked target: %w", errors.Join(ErrDenied, err))
	}
	return nil
}

func (e *Executor) openBuiltin(request ExecRequest) (*os.File, builtin, error) {
	b, err := parseBuiltin(request)
	if err != nil {
		return nil, b, err
	}
	path, err := e.relativePath(b.path)
	if err != nil {
		return nil, b, err
	}
	root, err := e.openRoot()
	if err != nil {
		return nil, b, err
	}
	file, err := openTarget(root, path, request.Command == builtinPWD || request.Command == builtinLS)
	if closeErr := root.Close(); closeErr != nil {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		return nil, b, errors.Join(ErrDenied, err, closeErr)
	}
	if err != nil {
		return nil, b, errors.Join(ErrDenied, err)
	}
	return file, b, nil
}

func (e *Executor) relativePath(path string) (string, error) {
	if e == nil || e.identity == nil || path == "" || len(path) > maxPath || strings.ContainsRune(path, 0) || hasTraversal(path) {
		return "", ErrDenied
	}
	for component := range strings.SplitSeq(filepath.ToSlash(e.root), "/") {
		if sensitive(component) {
			return "", ErrDenied
		}
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(e.root, path)
		if err != nil || !filepath.IsLocal(relative) {
			return "", ErrDenied
		}
		path = relative
	}
	if !filepath.IsLocal(path) {
		return "", ErrDenied
	}
	components := strings.Split(filepath.ToSlash(path), "/")
	if len(components) > maxComponents {
		return "", ErrDenied
	}
	for _, component := range components {
		if ambiguousComponent(component) || sensitive(component) {
			return "", ErrDenied
		}
	}
	return filepath.Clean(path), nil
}

func ambiguousComponent(name string) bool {
	if name == "." || name == "" {
		return false
	}
	return strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\\:") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ")
}

func hasTraversal(path string) bool {
	for component := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if component == ".." {
			return true
		}
	}
	return false
}

func sensitive(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, ".env") {
		return true
	}
	for _, marker := range []string{"credential", "secret", "password", "token", "privatekey", "private_key", "private-key", "keys"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if slices.Contains([]string{".git", ".ssh", ".aws", ".kube", ".azure", ".gnupg", ".zev"}, lower) {
		return true
	}
	return strings.HasPrefix(lower, "id_") || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx")
}

func (e *Executor) openRoot() (*os.Root, error) {
	if e == nil || e.identity == nil || e.pinned == nil {
		return nil, ErrDenied
	}
	if _, err := e.pinned.Stat("."); err != nil {
		return nil, errors.Join(ErrDenied, err)
	}
	root, err := openAbsoluteRoot(e.root)
	if err != nil {
		return nil, errors.Join(ErrDenied, err)
	}
	info, err := root.Stat(".")
	if err != nil || !os.SameFile(e.identity, info) {
		return nil, errors.Join(ErrDenied, err, root.Close())
	}
	return root, nil
}

func openAbsoluteRoot(path string) (*os.Root, error) {
	volume := filepath.VolumeName(path)
	root, err := os.OpenRoot(volume + string(filepath.Separator))
	if err != nil {
		return nil, fmt.Errorf("open filesystem root: %w", err)
	}
	components := strings.Split(strings.TrimPrefix(path[len(volume):], string(filepath.Separator)), string(filepath.Separator))
	if len(components) > maxComponents {
		return nil, errors.Join(ErrDenied, root.Close())
	}
	for _, component := range components {
		if component == "" {
			continue
		}
		next, openErr := openDirectory(root, component)
		closeErr := root.Close()
		if openErr != nil || closeErr != nil {
			if next != nil {
				closeErr = errors.Join(closeErr, next.Close())
			}
			return nil, errors.Join(ErrDenied, openErr, closeErr)
		}
		root = next
	}
	return root, nil
}

func openDirectory(root *os.Root, name string) (*os.Root, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect directory: %w", err)
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrDenied
	}
	next, err := root.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open directory: %w", err)
	}
	after, err := next.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		return nil, errors.Join(ErrDenied, err, next.Close())
	}
	return next, nil
}

func openTarget(root *os.Root, path string, directory bool) (*os.File, error) {
	parent := filepath.Dir(path)
	if parent != "." {
		child, err := openDirectory(root, strings.Split(parent, string(filepath.Separator))[0])
		if err != nil {
			return nil, err
		}
		remaining := strings.TrimPrefix(path, strings.Split(parent, string(filepath.Separator))[0]+string(filepath.Separator))
		file, openErr := openTarget(child, remaining, directory)
		closeErr := child.Close()
		if closeErr != nil {
			if file != nil {
				closeErr = errors.Join(closeErr, file.Close())
			}
			return nil, errors.Join(openErr, closeErr)
		}
		return file, openErr
	}
	before, err := root.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect target: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || (directory && !before.IsDir()) || (!directory && !before.Mode().IsRegular()) {
		return nil, ErrDenied
	}
	file, err := root.OpenFile(path, os.O_RDONLY|readFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("open target: %w", err)
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || (directory && !after.IsDir()) || (!directory && (!after.Mode().IsRegular() || multipleLinks(file, after))) {
		return nil, errors.Join(ErrDenied, err, file.Close())
	}
	return file, nil
}

func readBuiltin(ctx context.Context, file *os.File, request ExecRequest, b builtin, out *boundedOutput) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read builtin: %w", err)
	}
	if request.Command == builtinLS {
		return listDirectory(ctx, file, out)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxOutput+1))
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read builtin: %w", err)
	}
	if request.Command == builtinHead {
		lines := 0
		for i, c := range data {
			if c == '\n' {
				lines++
				if lines == b.lines {
					data = data[:i+1]
					break
				}
			}
		}
	}
	_, err = out.Write(data)
	return err
}

func listDirectory(ctx context.Context, file *os.File, out *boundedOutput) error {
	for range maxEntries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("list directory: %w", err)
		}
		entries, err := file.ReadDir(1)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read directory: %w", err)
		}
		name := entries[0].Name()
		if sensitive(name) || ambiguousComponent(name) || entries[0].Type()&os.ModeSymlink != 0 {
			continue
		}
		line := strconv.QuoteToASCII(name) + "\n"
		// Keep the filename's quoting intact at the output boundary.
		if len(line) > maxOutput-len(out.data) {
			out.truncated = true
			return nil
		}
		if _, err := out.Write([]byte(line)); err != nil {
			return err
		}
	}
	// The entry budget bounds work even if all entries are filtered out.
	out.truncated = true
	return nil
}
