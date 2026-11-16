package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestScriptShowPrintsExactBodyWithoutExecutingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(filepath.Join(dir, "script_files"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "#!/usr/bin/env bash\nprintf 'marker' > " + filepath.Join(home, "marker") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "scripts"), []byte("safe:\n  script: safe.sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script_files", "safe.sh"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}

	cmd := newScriptShowCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"safe"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != body {
		t.Fatalf("show output = %q, want %q", got, body)
	}
	if _, err := os.Lstat(filepath.Join(home, "marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("show executed script marker: %v", err)
	}
}

func TestScriptShowMissingReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cmd := newScriptShowCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"missing"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("show missing script succeeded")
	}
}

func TestScriptShowRequiresExactlyOneArgument(t *testing.T) {
	for _, args := range [][]string{nil, {"one", "two"}} {
		t.Run(filepath.Join(args...), func(t *testing.T) {
			cmd := newScriptShowCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil {
				t.Fatalf("show with args %v succeeded", args)
			}
		})
	}
}

type showFailingOutput struct{ err error }

func (w showFailingOutput) Write([]byte) (int, error) { return 0, w.err }

func TestScriptShowReturnsOutputError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".zev")
	if err := os.MkdirAll(filepath.Join(dir, "script_files"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts"), []byte("safe:\n  script: safe.sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script_files", "safe.sh"), []byte("body"), 0o700); err != nil {
		t.Fatal(err)
	}
	outputErr := errors.New("output unavailable")
	cmd := newScriptShowCmd()
	cmd.SetOut(showFailingOutput{err: outputErr})
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"safe"})

	if err := cmd.Execute(); !errors.Is(err, outputErr) {
		t.Fatalf("show error = %v, want output error", err)
	}
}
