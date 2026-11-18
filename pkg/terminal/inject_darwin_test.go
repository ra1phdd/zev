//go:build darwin

package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestInjectWritesToStdinFD verifies that the Darwin TIOCSTI request reaches
// the kernel: bytes injected into the TTY slave's input queue are readable
// from the PTY master. It fails with ENOTTY when the request value uses the
// Linux encoding (0x5412) instead of the Darwin _IOW('t', 114, char).
//
// The kernel permits TIOCSTI only on the caller's controlling terminal, so
// the injection runs in a session-leading child process whose controlling
// TTY is the PTY slave.
func TestInjectWritesToStdinFD(t *testing.T) { //nolint:paralleltest // Uses a dedicated PTY and child process.
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := master.Close(); err != nil {
			t.Error(err)
		}
	})

	const command = "echo Привет 世界"
	child := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestInjectHelperProcess", "--", command) //nolint:gosec // Re-execs the test binary itself; arguments are fixed by the test. The context does not cancel: the child exits on its own and the parent bounds the wait.
	child.Env = append(os.Environ(),
		"ZEV_INJECT_HELPER=1",
		"ZEV_INJECT_SLAVE_NAME="+slave.Name(),
	)
	child.Stdout = os.Stderr
	child.Stderr = os.Stderr
	child.SysProcAttr = sysProcAttrSetsid()

	if err := child.Start(); err != nil {
		t.Fatalf("child.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := slave.Close(); err != nil {
			t.Error(err)
		}
	})

	got := readMaster(t, master, len(command))
	if err := child.Wait(); err != nil {
		t.Fatalf("child.Wait() error = %v (master read %q)", err, got)
	}
	if got != command {
		t.Errorf("injected bytes = %q, want %q", got, command)
	}
}

// TestInjectHelperProcess is not a test: it is the child process entry point
// for TestInjectWritesToStdinFD. It runs as a session leader with the PTY
// slave as its controlling TTY and injects the argument into stdin.
func TestInjectHelperProcess(_ *testing.T) { //nolint:paralleltest // The helper must return immediately in the parent run and must os.Exit in the child; t.Parallel is meaningless in both.
	if os.Getenv("ZEV_INJECT_HELPER") != "1" {
		return
	}

	args := os.Args
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			os.Exit(injectHelperRun(args[i+1]))
		}
	}
	os.Exit(1)
}

// injectHelperRun opens the PTY slave as stdin and injects text. It returns
// the child process exit code instead of an error because the helper
// terminates via os.Exit and defers never run there.
func injectHelperRun(text string) int {
	slave, err := os.OpenFile(os.Getenv("ZEV_INJECT_SLAVE_NAME"), os.O_RDWR, 0) //nolint:gosec // The path is the PTY slave name passed by the parent test via its own environment.
	if err != nil {
		return 1
	}
	os.Stdin = slave

	if err := Inject(text); err != nil {
		// Best-effort close: the exit code already reports the failure.
		if cerr := slave.Close(); cerr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close slave: %v\n", cerr)
		}
		return 1
	}
	if err := slave.Close(); err != nil {
		return 1
	}
	return 0
}

// readMaster reads exactly size bytes from the PTY master. A goroutine
// performs the blocking read so the test can bound the wait with a timer.
func readMaster(t *testing.T, master *os.File, size int) string {
	t.Helper()
	type result struct {
		data string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, size)
		for sb.Len() < size {
			n, err := master.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if err != nil {
				done <- result{data: sb.String(), err: err}
				return
			}
		}
		done <- result{data: sb.String()}
	}()

	select {
	case res := <-done:
		if res.err != nil && !errors.Is(res.err, io.EOF) {
			t.Fatalf("Read() error = %v (read %q)", res.err, res.data)
		}
		return res.data
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for injected bytes on the PTY master")
		return ""
	}
}
