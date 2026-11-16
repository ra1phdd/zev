// Package childprocess runs commands with cancellation-aware cleanup.
package childprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Run starts cmd, owns its descendants, and waits for it to exit. Cancellation
// and child exit errors remain available through errors.Is and errors.As.
// cmd must not be started; Run replaces its Cancel callback and supplies a
// one-second WaitDelay if unset, bounding inherited pipe cleanup (not arbitrary
// blocking Readers or Writers supplied by callers).
// Remaining owned descendants are killed even on normal leader exit. Unix
// children must not detach into another process group or session. Windows uses
// a non-inheritable kill-on-close job; children are resumed only after assignment.
// Processes created through external brokers are outside this ownership boundary.
func Run(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}
	control, err := prepareCommand(cmd)
	if err != nil {
		return err
	}
	return runPrepared(ctx, cmd, control)
}

type processControl struct {
	started    func() error
	waitExited func() error
	kill       func() error
	close      func() error
}

func runPrepared(ctx context.Context, cmd *exec.Cmd, control processControl) (err error) {
	defer func() { err = errors.Join(err, control.close()) }()
	commandCanceled := make(chan struct{}, 1)
	// CommandContext must not kill the leader before group cleanup. Its watcher
	// only requests cancellation; this goroutine alone signals and then reaps.
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			select {
			case commandCanceled <- struct{}{}:
			default:
			}
			return nil
		}
	}
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = time.Second
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}
	if err := control.started(); err != nil {
		killErr := control.kill()
		// Assignment may have failed before the Windows job owned the child.
		processErr := cmd.Process.Kill()
		if errors.Is(processErr, os.ErrProcessDone) {
			processErr = nil
		}
		return errors.Join(err, killErr, processErr, cmd.Wait(), ctx.Err())
	}

	exited := make(chan error, 1)
	go func() { exited <- control.waitExited() }()
	var observeErr error
	var observed bool
	select {
	case observeErr = <-exited:
		observed = true
	case <-ctx.Done():
	case <-commandCanceled:
	}
	// Exit observation does not reap: the leader's identity pins its PGID until
	// the final group signal. No numeric PID/PGID signal occurs after cmd.Wait.
	killErr := control.kill()
	if !observed {
		observeErr = <-exited
	}
	return errors.Join(ctx.Err(), observeErr, killErr, cmd.Wait())
}
