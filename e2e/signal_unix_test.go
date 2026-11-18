//go:build e2e && !windows

package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCLIInterruptCancelsHTTP(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	canceled := make(chan struct{})
	shutdown := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read fixture request: %v", err)
			return
		}
		close(started)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-shutdown:
		}
	}))
	t.Cleanup(func() {
		close(shutdown)
		server.CloseClientConnections()
		server.Close()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, cliBinary, "--no-history", "--no-context", "signal fixture")
	home := t.TempDir()
	cmd.Dir = home
	cmd.Env = isolatedEnv(t, home)
	writeFixture(t, home, cliFixture{baseURL: server.URL, key: "signal-secret-marker"})
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Errorf("kill signal helper: %v", err)
			}
			if err := cmd.Wait(); err != nil {
				exitErr, exited := errors.AsType[*exec.ExitError](err)
				if !exited || exitErr == nil {
					t.Errorf("reap signal helper: %v", err)
				}
			}
		}
	})
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("CLI did not start request")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	if err != nil {
		exitErr, exited := errors.AsType[*exec.ExitError](err)
		if !exited || exitErr == nil {
			t.Fatal(err)
		}
	}
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal("signal did not cancel HTTP request")
	}
	if ctx.Err() != nil {
		t.Fatal("CLI failed to exit in time")
	}
	if strings.Contains(output.String(), "signal-secret-marker") {
		t.Fatal("CLI diagnostic leaked credential")
	}
}
