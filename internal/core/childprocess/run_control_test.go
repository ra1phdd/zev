package childprocess

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestRunPreparedReleasesControlAfterStartFailure(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("close ownership failure")
	closed := false
	control := processControl{close: func() error { closed = true; return closeErr }}
	cmd := exec.CommandContext(t.Context(), "./nonexistent-childprocess-test-executable")
	err := runPrepared(t.Context(), cmd, control)
	if !closed || !errors.Is(err, closeErr) {
		t.Fatalf("runPrepared() = %v, closed = %v", err, closed)
	}
}

func TestRunPreparedKillsAndReapsOnOwnershipFailure(t *testing.T) {
	t.Parallel()
	assignmentErr := errors.New("assignment failed")
	var events []string
	control := processControl{
		started: func() error { events = append(events, "assign"); return assignmentErr },
		kill:    func() error { events = append(events, "kill"); return nil },
		close:   func() error { events = append(events, "close"); return nil },
	}
	cmd := helperCommand(t, childRole)
	err := runPrepared(t.Context(), cmd, control)
	if !errors.Is(err, assignmentErr) {
		t.Fatalf("runPrepared() = %v, want assignment error", err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("failed ownership left child unreaped")
	}
	if !reflect.DeepEqual(events, []string{"assign", "kill", "close"}) {
		t.Fatalf("ownership cleanup order = %v", events)
	}
}

func TestRunPreparedSignalsBeforeWait(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cmd := helperCommandContext(ctx, t, childRole, "")
	var events []string
	stopObserver := make(chan struct{})
	control := processControl{
		started:    func() error { events = append(events, "start"); cancel(); return nil },
		waitExited: func() error { <-stopObserver; return nil },
		kill: func() error {
			events = append(events, "kill")
			if cmd.ProcessState != nil {
				t.Error("child reaped before ownership signal")
			}
			close(stopObserver)
			return cmd.Process.Kill()
		},
		close: func() error { events = append(events, "close"); return nil },
	}
	err := runPrepared(ctx, cmd, control)
	if !errors.Is(err, context.Canceled) || cmd.ProcessState == nil {
		t.Fatalf("runPrepared() = %v, state = %v", err, cmd.ProcessState)
	}
	if !reflect.DeepEqual(events, []string{"start", "kill", "close"}) {
		t.Fatalf("ownership cleanup order = %v", events)
	}
	if err := cmd.Process.Kill(); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("reaped child Kill() = %v", err)
	}
}
