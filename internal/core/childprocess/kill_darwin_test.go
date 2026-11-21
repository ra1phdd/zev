//go:build darwin

package childprocess

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinGroupSignalPreservesPermissionErrors(t *testing.T) {
	t.Parallel()
	leader := unix.KinfoProc{Proc: unix.ExternProc{P_pid: 42, P_stat: 5}}
	live := unix.KinfoProc{Proc: unix.ExternProc{P_pid: 43, P_stat: 2}}
	queryErr := errors.New("process table unavailable")
	for _, test := range []struct {
		name      string
		members   []unix.KinfoProc
		queryErr  error
		wantError bool
	}{
		{"zombie leader only", []unix.KinfoProc{leader}, nil, false},
		{"live descendant", []unix.KinfoProc{leader, live}, nil, true},
		{"empty group", nil, nil, false},
		{"missing leader", []unix.KinfoProc{{Proc: unix.ExternProc{P_pid: 43, P_stat: 5}}}, nil, true},
		{"failed observation", []unix.KinfoProc{leader}, queryErr, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := classifyGroupSignal(42, syscall.EPERM, test.members, test.queryErr)
			if errors.Is(err, syscall.EPERM) != test.wantError {
				t.Errorf("classifyGroupSignal() = %v, wantError %v", err, test.wantError)
			}
			if test.queryErr != nil && !errors.Is(err, test.queryErr) {
				t.Errorf("classifyGroupSignal() lost observation error: %v", err)
			}
		})
	}
}

func TestDarwinKillProcessAllowsAnExitedLeaderWithAnEmptyGroup(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "true")
	if _, err := prepareCommand(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := waitForExit(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if err := killProcess(cmd); err != nil {
		if waitErr := cmd.Wait(); waitErr != nil {
			t.Errorf("reap exited leader: %v", waitErr)
		}
		t.Fatalf("killProcess() = %v, want no error for an already exited leader", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
