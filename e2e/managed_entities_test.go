//go:build e2e && !windows

package e2e_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

const (
	managedE2EHelpFlag   = "--help"
	managedE2EJSONFlag   = "--json"
	managedE2EPrintFlag  = "--print"
	managedE2EShellEnv   = "SHELL"
	managedE2EExitScript = "exitseven"
)

func TestManagedOriginalArgvNative(t *testing.T) {
	t.Parallel()
	home := fixtureHome(t)
	body := filepath.Join(home, "capture.sh")
	if err := os.WriteFile(body, []byte("#!/bin/sh\nprintf '%s\\0' \"$@\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	add := cliCommand(t, home, "alias", "add", "capture", "sh "+body+" {{.args}}", "--no-metadata")
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("add: %v %s", err, output)
	}
	marker := filepath.Join(home, "marker")
	argv := []string{"a b", "", "x\ty", "x\ny", "Юникод", "'\"", "$HOME", "*", "; touch " + marker, "&& touch " + marker, "$(touch " + marker + ")", "`touch " + marker + "`", managedE2EJSONFlag, "--context-file", "missing", "--", managedE2EHelpFlag, "--"}
	cmd := cliCommand(t, home, append([]string{"capture"}, argv...)...)
	writeFixture(t, home, cliFixture{toolApproval: "manual", securityMode: config.SecurityModeBypass})
	setCLIEnv(cmd, map[string]string{managedE2EShellEnv: "/bin/sh"})
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v %s", err, stderr.String())
	}
	expected := append(append([]string{}, argv[:15]...), argv[16:]...)
	want := strings.Join(expected, "\x00") + "\x00"
	if string(output) != want {
		t.Fatalf("argv %q want %q", output, want)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("metacharacters executed: %v", err)
	}
	for _, flag := range []string{managedE2EPrintFlag, managedE2EJSONFlag} {
		cmd := cliCommand(t, home, flag, "capture", "$(touch "+marker+")")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("machine: %v %s", err, output)
		}
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("machine mode executed")
	}
}

func TestManagedAliasRejectsUnsupportedShellSyntax(t *testing.T) {
	t.Parallel()
	home := fixtureHome(t)
	add := cliCommand(t, home, "alias", "add", "unsafe", "eval {{.arg1}}", "--no-metadata")
	if output, err := add.CombinedOutput(); err == nil {
		t.Fatalf("unsupported shell alias was accepted: %s", output)
	}
}

func TestManagedNativeExitCodesAndHelp(t *testing.T) {
	t.Parallel()
	home := fixtureHome(t)
	create := cliCommand(t, home, "script", "create", managedE2EExitScript, "--body-stdin", "--argument", "target", managedE2EJSONFlag)
	create.Stdin = strings.NewReader("#!/bin/bash\nexit 7\n")
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create: %v %s", err, output)
	}
	for _, tt := range []struct {
		args []string
		code int
	}{
		{[]string{managedE2EExitScript, managedE2EHelpFlag}, 0},
		{[]string{managedE2EExitScript}, 2},
		{[]string{managedE2EExitScript, "present"}, 7},
		{[]string{managedE2EPrintFlag, managedE2EExitScript}, 2},
		{[]string{managedE2EExitScript, "present", managedE2EHelpFlag}, 2},
	} {
		cmd := cliCommand(t, home, tt.args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			exit, ok := errors.AsType[*exec.ExitError](err)
			if !ok {
				t.Fatal(err)
			}
			code = exit.ExitCode()
		}
		if code != tt.code {
			t.Errorf("%v: code %d want %d: %s", tt.args, code, tt.code, stderr.String())
		}
		if tt.code == 2 && stdout.Len() != 0 {
			t.Errorf("failure stdout: %s", stdout.String())
		}
	}
}
