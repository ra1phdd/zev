package repository

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newExecutor(t *testing.T) (*Executor, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	return e, root
}

const readmeName = "readme"

func TestReadOnlyBuiltins(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	if err := os.WriteFile(filepath.Join(root, readmeName), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		request ExecRequest
		want    string
	}{
		{ExecRequest{Command: builtinPWD}, root + "\n"},
		{ExecRequest{Command: builtinLS}, "\"readme\"\n"},
		{ExecRequest{Command: builtinCat, Args: []string{readmeName}}, "one\ntwo\nthree\n"},
		{ExecRequest{Command: builtinHead, Args: []string{"-n", "2", readmeName}}, "one\ntwo\n"},
	}
	for _, tt := range tests {
		t.Run(tt.request.Command, func(t *testing.T) {
			if err := e.CheckReadOnly(tt.request); err != nil {
				t.Fatal(err)
			}
			result := e.Execute(t.Context(), tt.request, true)
			if result.Denied || result.ExitCode != 0 || result.Output != tt.want || result.Truncated {
				t.Fatalf("Execute() = %+v; want %q", result, tt.want)
			}
		})
	}
}

func TestReadOnlySensitiveComponents(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	for _, name := range []string{".env", ".environment", ".ENV.prod", "credentials", "secrets", "passwords", "Tokens", "privatekey", "private_key", "keys", ".git", ".ssh", ".aws", ".kube", ".azure", ".gnupg", ".zev", "id_rsa", "cert.pem", "cert.key", "bundle.p12", "bundle.pfx"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "safe"), []byte("hidden"), 0o600); err != nil {
			t.Fatal(err)
		}
		request := ExecRequest{Command: builtinCat, Args: []string{name + "/safe"}}
		if err := e.CheckReadOnly(request); err == nil {
			t.Errorf("accepted protected component %q", name)
		}
		if result := e.Execute(t.Context(), request, true); !result.Denied || result.Output != "" {
			t.Errorf("read protected component %q: %+v", name, result)
		}
	}
	result := e.Execute(t.Context(), ExecRequest{Command: builtinLS}, true)
	if result.Output != "" || result.ExitCode != 0 {
		t.Fatalf("listed sensitive entries: %+v", result)
	}
}

func TestReadOnlySymlinksAndRevalidation(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "data"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		if runtime.GOOS == windowsOS {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if err := os.Symlink("safe", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"linked/data", "alias"} {
		request := ExecRequest{Command: builtinCat, Args: []string{path}}
		if err := e.CheckReadOnly(request); err == nil {
			t.Fatalf("accepted symlink %q", path)
		}
		if result := e.Execute(t.Context(), request, true); !result.Denied {
			t.Fatalf("read symlink: %+v", result)
		}
	}
	request := ExecRequest{Command: builtinCat, Args: []string{"safe"}}
	if err := e.CheckReadOnly(request); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "safe"), filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "data"), filepath.Join(root, "safe")); err != nil {
		t.Fatal(err)
	}
	if result := e.Execute(t.Context(), request, true); !result.Denied || result.Output != "" {
		t.Fatalf("did not revalidate: %+v", result)
	}
	if _, err := New(filepath.Join(root, "linked")); err == nil {
		t.Fatal("accepted symlink root")
	}
	if _, err := New(filepath.Join(root, "linked", "nested")); err == nil {
		t.Fatal("accepted symlink ancestor")
	}
}

func TestReadOnlyLimitsAndQuotedNames(t *testing.T) {
	t.Parallel()
	e, root := newExecutor(t)
	name := "line\n\x1b[31m\"$(touch marker)"
	if runtime.GOOS == windowsOS {
		name = "file$(touch marker)"
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte(strings.Repeat("x", 2*maxOutput)), 0o600); err != nil {
		t.Fatal(err)
	}
	result := e.Execute(t.Context(), ExecRequest{Command: builtinCat, Args: []string{name}}, true)
	if result.ExitCode != 0 || !result.Truncated || len(result.Output) != maxOutput {
		t.Fatalf("unbounded read: %+v", result)
	}
	result = e.Execute(t.Context(), ExecRequest{Command: builtinLS}, true)
	if result.ExitCode != 0 || strings.ContainsRune(result.Output, '\x1b') || strings.Count(result.Output, "\n") != 1 || !strings.HasPrefix(result.Output, "\"") {
		t.Fatalf("unquoted filename: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "marker")); !os.IsNotExist(err) {
		t.Fatal("filename was evaluated")
	}
}

func TestReadOnlyRejectsUnsupportedRequests(t *testing.T) {
	t.Parallel()
	e, _ := newExecutor(t)
	for _, request := range []ExecRequest{
		{Command: "sh", Args: []string{"-c", builtinPWD}},
		{Command: "/bin/cat", Args: []string{readmeName}},
		{Command: "cat readme"},
		{Command: "git", Args: []string{"status"}},
		{Command: builtinPWD, Args: []string{"-P"}},
		{Command: builtinLS, Args: []string{"-R"}},
		{Command: builtinCat},
		{Command: builtinCat, Args: []string{"--", readmeName}},
		{Command: builtinHead, Args: []string{"-c", "1", readmeName}},
		{Command: builtinHead, Args: []string{"-n", "1001", readmeName}},
		{Command: builtinCat, Args: []string{"../outside"}},
		{Command: builtinLS, Args: []string{"x/../"}},
		{Command: builtinCat, Args: []string{"/etc/passwd"}},
		{Command: builtinLS, Args: []string{"-file"}},
	} {
		t.Run(strings.Join(append([]string{request.Command}, request.Args...), " "), func(t *testing.T) {
			if err := e.CheckReadOnly(request); err == nil {
				t.Fatal("CheckReadOnly accepted unsupported request")
			}
			if result := e.Execute(t.Context(), request, true); !result.Denied || result.ExitCode == 0 {
				t.Fatalf("Execute accepted unsupported request: %+v", result)
			}
		})
	}
}
