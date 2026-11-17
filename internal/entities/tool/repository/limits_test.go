package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestNestedAndAbsoluteReads(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	directory := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "text")
	text := strings.Repeat("line\n", 20)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, request := range []ExecRequest{
		{Command: builtinCat, Args: []string{"a/b/text"}},
		{Command: builtinCat, Args: []string{path}},
		{Command: builtinHead, Args: []string{"a/b/text"}},
		{Command: builtinLS, Args: []string{"a/b"}},
	} {
		if err := e.CheckReadOnly(request); err != nil {
			t.Fatal(err)
		}
		result := e.Execute(t.Context(), request, true)
		want := text
		switch request.Command {
		case builtinHead:
			want = strings.Repeat("line\n", 10)
		case builtinLS:
			want = "\"text\"\n"
		}
		if result.Output != want || result.ExitCode != 0 {
			t.Fatalf("Execute(%+v) = %+v", request, result)
		}
	}
}

func TestRootReplacementAndZeroValue(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	moved := root + "-moved"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(moved); err != nil {
			t.Error(err)
		}
	})
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, executor := range []*Executor{e, {}, nil} {
		if err := executor.CheckReadOnly(ExecRequest{Command: builtinPWD}); !errors.Is(err, ErrDenied) {
			t.Fatalf("CheckReadOnly() = %v", err)
		}
		for _, restricted := range []bool{true, false} {
			if result := executor.Execute(t.Context(), ExecRequest{Command: builtinPWD}, restricted); !result.Denied {
				t.Fatalf("invalid root executed: %+v", result)
			}
		}
	}
	for _, path := range []string{"", filepath.Join(root, "missing"), root + "/../", strings.Repeat("x", maxPath+1)} {
		if _, err := New(path); err == nil {
			t.Fatalf("New(%q) accepted invalid root", path)
		}
	}
}

func TestClosedExecutorDeniesNewOperations(t *testing.T) {
	t.Parallel()
	e, _ := newExecutor(t)
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.CheckReadOnly(ExecRequest{Command: builtinPWD}); !errors.Is(err, ErrDenied) {
		t.Fatalf("closed CheckReadOnly = %v", err)
	}
	if result := e.Execute(t.Context(), ExecRequest{Command: builtinPWD}, true); !result.Denied {
		t.Fatalf("closed executor ran: %+v", result)
	}
	if err := (*Executor)(nil).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryEntryBudget(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	for i := range maxEntries + 2 {
		path := filepath.Join(root, fmt.Sprintf("file%04d", i))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result := e.Execute(t.Context(), ExecRequest{Command: builtinLS}, true)
	if result.ExitCode != 0 || !result.Truncated || strings.Count(result.Output, "\n") != maxEntries {
		t.Fatalf("entry budget: exit=%d truncated=%v lines=%d", result.ExitCode, result.Truncated, strings.Count(result.Output, "\n"))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rootFile, err := e.pinned.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	var out boundedOutput
	readErr := listDirectory(ctx, rootFile, &out)
	closeErr := rootFile.Close()
	if !errors.Is(readErr, context.Canceled) || closeErr != nil {
		t.Fatalf("list cancellation: %v, close: %v", readErr, closeErr)
	}
}

func TestListingTruncatesBetweenQuotedNames(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	for i := range 200 {
		name := fmt.Sprintf("%03d-", i) + strings.Repeat("x", 196)
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result := e.Execute(t.Context(), ExecRequest{Command: builtinLS}, true)
	if !result.Truncated || !strings.HasSuffix(result.Output, "\n") {
		t.Fatalf("listing split a quoted name: bytes=%d truncated=%v", len(result.Output), result.Truncated)
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(result.Output, "\n"), "\n") {
		if _, err := strconv.Unquote(line); err != nil {
			t.Fatalf("invalid quoted filename %q: %v", line, err)
		}
	}
}

func TestProtectedRootDenied(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "credentials", "project")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := e.CheckReadOnly(ExecRequest{Command: builtinPWD}); !errors.Is(err, ErrDenied) {
		t.Fatalf("accepted protected root ancestor: %v", err)
	}
}

func TestPlatformAmbiguousComponentsDenied(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	for _, name := range []string{".git.", ".ssh ", "safe."} {
		if runtime.GOOS == windowsOS {
			continue
		}
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := e.CheckReadOnly(ExecRequest{Command: builtinLS, Args: []string{name}}); !errors.Is(err, ErrDenied) {
			t.Errorf("accepted ambiguous component %q", name)
		}
	}
}

func TestRequestBudgetsAndFlagInjection(t *testing.T) {
	t.Parallel()
	e, _ := newExecutor(t)
	for _, request := range []ExecRequest{
		{Command: builtinCat, Args: []string{"."}},
		{Command: builtinHead, Args: []string{"-n", "0", "."}},
		{Command: builtinHead, Args: []string{"-n", "01", "."}},
		{Command: builtinHead, Args: []string{"-n", "+1", "."}},
		{Command: builtinHead, Args: []string{"-n", strings.Repeat("1", 20000), "."}},
		{Command: builtinLS, Args: []string{"./-R"}},
		{Command: builtinLS, Args: []string{"a/-R"}},
		{Command: builtinCat, Args: []string{"foo\x00bar"}},
		{Command: builtinCat, Args: []string{"dir\\file"}},
		{Command: builtinCat, Args: []string{"file:stream"}},
		{Command: builtinCat, Args: []string{strings.Repeat("./", maxComponents+1) + "file"}},
		{Command: builtinLS, Reason: strings.Repeat("r", 2049)},
	} {
		if err := e.CheckReadOnly(request); !errors.Is(err, ErrDenied) {
			t.Fatalf("accepted invalid request: %+v, error=%v", request, err)
		}
	}
	for _, request := range []ExecRequest{
		{},
		{Command: "x\x00"},
		{Command: strings.Repeat("x", maxPath+1)},
		{Command: "x", Args: make([]string, 65)},
		{Command: "x", Args: []string{strings.Repeat("x", maxPath+1)}},
		{Command: "x", Args: []string{strings.Repeat("x", maxPath), strings.Repeat("x", maxPath), strings.Repeat("x", maxPath), strings.Repeat("x", maxPath)}},
	} {
		if result := e.Execute(t.Context(), request, false); !result.Denied {
			t.Fatalf("accepted over-budget request: %+v", result)
		}
	}
}
