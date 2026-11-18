//go:build e2e && !windows

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const aliasPOSIXShell = "/bin/sh"

func TestManagedAliasNativeExecutionBranches(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, shell, template, argument, wantOutput, wantError string
		wantCode                                               int
	}{
		{name: "default shell", template: metadataTemplate, argument: "literal $HOME; * ' Юникод", wantOutput: "literal $HOME; * ' Юникод"},
		{name: "explicit POSIX shell", shell: aliasPOSIXShell, template: metadataTemplate, argument: "two words", wantOutput: "two words"},
		{name: "unsupported shell", shell: "/bin/fish", template: metadataTemplate, argument: "must-not-print", wantCode: 1, wantError: "unsupported alias shell"},
		{name: "shell start failure", shell: "/zev-nonexistent-fixture/sh", template: metadataTemplate, argument: "must-not-print", wantCode: 1, wantError: "start process"},
		{name: "child exit status", template: "exit {{.arg1}}", argument: "7", wantCode: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := fixtureHome(t)
			add := cliCommand(t, home, "alias", "add", "native", tc.template, "--no-metadata")
			if output, err := add.CombinedOutput(); err != nil {
				t.Fatalf("add native alias: %v %s", err, output)
			}
			cmd := cliCommand(t, home, "native", tc.argument)
			setCLIEnv(cmd, map[string]string{managedE2EShellEnv: tc.shell})
			var out, diagnostic bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &diagnostic
			err := cmd.Run()
			code := 0
			if err != nil {
				exit, ok := errors.AsType[*exec.ExitError](err)
				if !ok {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != tc.wantCode || out.String() != tc.wantOutput || !strings.Contains(diagnostic.String(), tc.wantError) {
				t.Fatalf("exit=%d stdout=%q stderr=%q; want exit=%d stdout=%q error=%q", code, out.String(), diagnostic.String(), tc.wantCode, tc.wantOutput, tc.wantError)
			}
		})
	}
}

func TestManagedAliasNativeInterruptIsBounded(t *testing.T) {
	t.Parallel()
	home := fixtureHome(t)
	body := filepath.Join(home, "wait.sh")
	pidPath := filepath.Join(home, "child.pid")
	parentPath := filepath.Join(home, "parent.pid")
	gate := filepath.Join(home, "child.start")
	fifo := filepath.Join(home, "wait.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	// The child blocks in a builtin with no descendants and retains its unique script argv for cleanup.
	// Its waiting parent prevents shell exec optimization from making it the managed group leader.
	fixture := `#!/bin/sh
if [ "$1" = child ]; then
  printf '%s\n' "$$" > "$2.tmp" && mv "$2.tmp" "$2"
  read -r ignored < "$3"
  exit
fi
printf '%s\n' "$$" > "$4.tmp" && mv "$4.tmp" "$4"
printf 'parent-ready\n'
read -r start < "$5"
sh "$0" child "$2" "$3" "$4" "$5" &
wait
`
	if err := os.WriteFile(body, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	add := cliCommand(t, home, "alias", "add", "waitnative", "sh {{.arg1}} {{.arg2}} {{.arg3}} {{.arg4}} {{.arg5}} {{.arg6}}", "--no-metadata")
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("add wait alias: %v %s", err, output)
	}
	cmd := cliCommand(t, home, "waitnative", body, "parent", pidPath, fifo, parentPath, gate)
	if err := syscall.Mkfifo(gate, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	// This bounds inherited diagnostic pipes only; child liveness is asserted independently.
	cmd.WaitDelay = time.Second
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	// Cleanup must survive every failed assertion, including a CLI that exits without killing its child.
	t.Cleanup(func() {
		if err := stdout.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close readiness pipe: %v", err)
		}
		select {
		case <-done:
		default:
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Errorf("kill CLI: %v", err)
			}
		}
		// Stop the parent before reading the child record, so it cannot launch a late child during cleanup.
		for _, path := range []string{parentPath, pidPath} {
			data, err := os.ReadFile(path) //nolint:gosec // Both PID paths are created in this test's private HOME.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				t.Errorf("read cleanup child PID: %v", err)
				continue
			}
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr != nil || pid <= 1 {
				t.Errorf("invalid cleanup child PID %q: %v", data, parseErr)
			} else if err := cleanupNativeAliasChild(pid, body); err != nil {
				t.Errorf("cleanup native child: %v", err)
			}
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("CLI cleanup did not reap within 5s")
		}
	})

	parentReady := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && line != "parent-ready\n" {
			err = fmt.Errorf("unexpected parent readiness %q", line)
		}
		parentReady <- err
	}()
	select {
	case err := <-parentReady:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("native parent did not publish readiness")
	}
	// No child can be launched before cleanup owns the parent and its readiness record.
	var gateWriter *os.File
	gateDeadline := time.Now().Add(5 * time.Second)
	for {
		gateWriter, err = os.OpenFile(gate, os.O_WRONLY|syscall.O_NONBLOCK, 0) //nolint:gosec // This FIFO belongs to the private fixture.
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.ENXIO) || time.Now().After(gateDeadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := gateWriter.WriteString("start\n"); err != nil {
		t.Error(err)
	}
	if err := gateWriter.Close(); err != nil {
		t.Fatal(err)
	}

	readyDeadline := time.Now().Add(5 * time.Second)
	var childPID int
	for childPID == 0 {
		data, err := os.ReadFile(pidPath) //nolint:gosec // pidPath is a file created in this test's private HOME.
		if err == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil || childPID <= 1 {
				t.Fatalf("invalid readiness child PID %q: %v", data, err)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(readyDeadline) {
			t.Fatal("native alias did not publish its child PID")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if group, err := syscall.Getpgid(childPID); err != nil || group == childPID || group == syscall.Getpgrp() {
		t.Fatalf("native child %d must be a descendant in an isolated group: pgid=%d err=%v", childPID, group, err)
	}
	if running, err := nativeAliasChildRunning(childPID, body); err != nil || !running {
		t.Fatalf("native child %d not running at readiness: running=%t err=%v", childPID, running, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	completion := time.NewTimer(time.Until(deadline))
	t.Cleanup(func() { completion.Stop() })
	select {
	case <-done:
	case <-completion.C:
		t.Fatal("CLI did not complete within 5s after interrupt")
	}
	if time.Now().After(deadline) {
		t.Fatal("CLI completion exceeded 5s after interrupt")
	}
	if exit, ok := errors.AsType[*exec.ExitError](waitErr); !ok || exit.ExitCode() == 0 {
		t.Fatalf("canceled alias returned %v; stderr=%s", waitErr, diagnostic.String())
	}
	if err := waitForNativeAliasChildExit(childPID, body, deadline); err != nil {
		t.Fatalf("native child %d remained after CLI completion: %v", childPID, err)
	}
}

func nativeAliasChildRunning(pid int, body string) (bool, error) {
	if pid <= 1 {
		return false, fmt.Errorf("invalid child PID %d", pid)
	}
	if err := syscall.Kill(pid, 0); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		return true, fmt.Errorf("probe child: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-ww", "-o", "stat=", "-o", "args=", "-p", strconv.Itoa(pid)) //nolint:gosec // The PID is numeric and comes from the private child fixture.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	output, err := cmd.Output()
	if err != nil {
		// A child can disappear between kill(0) and ps. Do not confuse other ps failures with exit.
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return false, nil
		}
		return true, fmt.Errorf("read child status: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return true, fmt.Errorf("missing child status: %q", output)
	}
	// An orphaned zombie cannot run; only its new parent can reap it on Linux/macOS.
	if strings.HasPrefix(fields[0], "Z") {
		return false, nil
	}
	if len(fields) < 2 {
		return true, fmt.Errorf("missing child argv: %q", output)
	}
	// The fixture never execs, so a different argv means this numeric PID was reused.
	return slices.Contains(fields[1:], body), nil
}

func waitForNativeAliasChildExit(pid int, body string, deadline time.Time) error {
	for {
		running, err := nativeAliasChildRunning(pid, body)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("child is still running at the completion deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func cleanupNativeAliasChild(pid int, body string) error {
	running, err := nativeAliasChildRunning(pid, body)
	if err != nil || !running {
		return err
	}
	// Signal only the identified fixture PID, never a possibly reused process group.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill child %d: %w", pid, err)
	}
	return waitForNativeAliasChildExit(pid, body, time.Now().Add(5*time.Second))
}
